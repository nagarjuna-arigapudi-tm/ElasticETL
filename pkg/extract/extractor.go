package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"elasticetl/pkg/config"
	"elasticetl/pkg/utils"

	"github.com/tidwall/gjson"
)

// substituteEnvVars replaces environment variables in the format ${VAR_NAME}
func substituteEnvVars(input string) string {
	re := regexp.MustCompile(`\$\{([^}]+)\}`)
	return re.ReplaceAllStringFunc(input, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := strings.TrimPrefix(strings.TrimSuffix(match, "}"), "${")
		if envValue := os.Getenv(varName); envValue != "" {
			return envValue
		}
		return match // Return original if env var not found
	})
}

// Result represents extracted data
type Result struct {
	Timestamp time.Time              `json:"timestamp"`
	Source    string                 `json:"source"`
	Data      map[string]interface{} `json:"data"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// Extractor handles data extraction from Elasticsearch
type Extractor struct {
	config           config.ExtractConfig
	httpClient       *http.Client
	macroSubstituter *utils.MacroSubstituter
	mutex            sync.RWMutex
}

// NewExtractor creates a new extractor
func NewExtractor(cfg config.ExtractConfig) *Extractor {
	macroSubstituter := utils.NewMacroSubstituter(cfg.StartTime, cfg.EndTime)
	return &Extractor{
		config:           cfg,
		macroSubstituter: macroSubstituter,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

// Extract performs data extraction from all configured endpoints
func (e *Extractor) Extract(ctx context.Context) ([]*Result, error) {
	var results []*Result
	var wg sync.WaitGroup

	// Calculate minimum length to avoid index out of bounds
	minLen := len(e.config.URLs)
	if len(e.config.ClusterNames) < minLen {
		minLen = len(e.config.ClusterNames)
	}
	if len(e.config.AuthHeaders) > 0 && len(e.config.AuthHeaders) < minLen {
		minLen = len(e.config.AuthHeaders)
	}
	if len(e.config.AdditionalHeaders) > 0 && len(e.config.AdditionalHeaders) < minLen {
		minLen = len(e.config.AdditionalHeaders)
	}

	resultsChan := make(chan *Result, minLen)
	errorsChan := make(chan error, minLen)

	// Extract from all endpoints concurrently
	for i := 0; i < minLen; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			result, err := e.extractFromEndpoint(ctx, index)
			if err != nil {
				errorsChan <- fmt.Errorf("endpoint %s: %w", e.config.URLs[index], err)
				return
			}

			if result != nil {
				resultsChan <- result
			}
		}(i)
	}

	// Wait for all extractions to complete
	go func() {
		wg.Wait()
		close(resultsChan)
		close(errorsChan)
	}()

	// Collect results and errors
	var errors []error
	for {
		select {
		case result, ok := <-resultsChan:
			if !ok {
				resultsChan = nil
			} else {
				results = append(results, result)
			}
		case err, ok := <-errorsChan:
			if !ok {
				errorsChan = nil
			} else {
				errors = append(errors, err)
			}
		}

		if resultsChan == nil && errorsChan == nil {
			break
		}
	}

	// Return error if all extractions failed
	if len(results) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("all extractions failed: %v", errors)
	}

	// Debug output after extract phase if enabled
	if e.config.Debug.Enabled && e.config.Debug.Path != "" {
		if err := e.writeDebugOutput(results); err != nil {
			fmt.Printf("Failed to write debug output: %v\n", err)
		}
	}

	return results, nil
}

// extractFromEndpoint extracts data from a single endpoint by index
func (e *Extractor) extractFromEndpoint(ctx context.Context, index int) (*Result, error) {
	url := e.config.URLs[index]
	clusterName := e.config.ClusterNames[index]

	// Substitute macros in the query
	processedQuery, err := e.macroSubstituter.SubstituteQuery(e.config.ElasticsearchQuery, clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to substitute macros in query: %w", err)
	}

	// Prepare Elasticsearch query - use raw query string directly
	req, err := http.NewRequestWithContext(ctx, "POST", url+"/_search", bytes.NewBufferString(processedQuery))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add auth header if provided (with environment variable substitution)
	if len(e.config.AuthHeaders) > index && e.config.AuthHeaders[index] != "" {
		authHeader := substituteEnvVars(e.config.AuthHeaders[index])
		req.Header.Set("Authorization", authHeader)
	}

	// Add additional headers if provided (with environment variable substitution)
	if len(e.config.AdditionalHeaders) > index && len(e.config.AdditionalHeaders[index]) > 0 {
		for _, header := range e.config.AdditionalHeaders[index] {
			// Each header should be in format "Key: Value"
			if len(header) > 0 {
				// Substitute environment variables in the header
				header = substituteEnvVars(header)

				// Split header string by first colon
				parts := strings.SplitN(header, ":", 2)
				if len(parts) == 2 {
					key := strings.TrimSpace(parts[0])
					value := strings.TrimSpace(parts[1])
					req.Header.Set(key, value)
				}
			}
		}
	}

	// Execute request with retries
	var resp *http.Response
	var lastErr error

	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		resp, lastErr = e.httpClient.Do(req)
		if lastErr == nil && resp.StatusCode < 500 {
			break
		}

		if resp != nil {
			resp.Body.Close()
		}

		if attempt < e.config.MaxRetries {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}

	if lastErr != nil {
		return nil, fmt.Errorf("request failed after %d retries: %w", e.config.MaxRetries, lastErr)
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Extract data using JSON paths
	extractedData, err := e.extractDataFromResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to extract data: %w", err)
	}

	result := &Result{
		Timestamp: time.Now(),
		Source:    url,
		Data:      extractedData,
		Metadata: map[string]interface{}{
			"endpoint":       url,
			"cluster_name":   clusterName,
			"query":          processedQuery,
			"original_query": e.config.ElasticsearchQuery,
			"response_size":  len(body),
		},
	}

	return result, nil
}

// extractDataFromResponse extracts data from Elasticsearch response using single JSON path and flattens it
func (e *Extractor) extractDataFromResponse(responseBody []byte) (map[string]interface{}, error) {
	if e.config.JSONPath == "" {
		// If no JSON path specified, return the entire response flattened
		var data interface{}
		if err := json.Unmarshal(responseBody, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return e.flattenJSON(data, ""), nil
	}

	responseStr := string(responseBody)
	result := gjson.Get(responseStr, e.config.JSONPath)

	if !result.Exists() {
		return make(map[string]interface{}), nil
	}

	// Parse the extracted JSON
	var extractedData interface{}
	if err := json.Unmarshal([]byte(result.Raw), &extractedData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal extracted JSON: %w", err)
	}

	// Flatten the extracted data
	flattened := e.flattenJSON(extractedData, "")

	// Apply filters
	filtered := e.applyFilters(flattened)

	return filtered, nil
}

// flattenJSON recursively flattens a JSON structure
func (e *Extractor) flattenJSON(data interface{}, prefix string) map[string]interface{} {
	result := make(map[string]interface{})

	switch v := data.(type) {
	case map[string]interface{}:
		// Handle single key-value pair with "value" key (case insensitive)
		if len(v) == 1 {
			for key, value := range v {
				if strings.ToLower(key) == "value" {
					// Assign the value to parent
					if prefix != "" {
						result[prefix] = value
					} else {
						result["value"] = value
					}
					return result
				}
			}
		}

		// Regular object flattening
		for key, value := range v {
			newKey := key
			if prefix != "" {
				newKey = prefix + "." + key
			}

			flattened := e.flattenJSON(value, newKey)
			for k, v := range flattened {
				result[k] = v
			}
		}

	case []interface{}:
		// Handle arrays - create multiple rows
		for i, item := range v {
			indexKey := fmt.Sprintf("%s[%d]", prefix, i)
			if prefix == "" {
				indexKey = fmt.Sprintf("[%d]", i)
			}

			flattened := e.flattenJSON(item, indexKey)
			for k, v := range flattened {
				result[k] = v
			}
		}

	default:
		// Primitive value
		if prefix != "" {
			result[prefix] = v
		} else {
			result["value"] = v
		}
	}

	return result
}

// applyFilters applies configured filters to flattened data
func (e *Extractor) applyFilters(data map[string]interface{}) map[string]interface{} {
	if len(e.config.Filters) == 0 {
		return data
	}

	result := make(map[string]interface{})

	// Check if we have include filters - if so, start with empty result
	hasIncludeFilters := false
	for _, filter := range e.config.Filters {
		if filter.Type == "include" {
			hasIncludeFilters = true
			break
		}
	}

	if !hasIncludeFilters {
		// Copy all data first if no include filters
		for k, v := range data {
			result[k] = v
		}
	}

	// Apply filters
	for _, filter := range e.config.Filters {
		if filter.Type == "exclude" {
			// Remove keys that match the filter
			for key := range result {
				if e.matchesFilter(key, filter.Pattern) {
					delete(result, key)
				}
			}
		} else if filter.Type == "include" {
			// Add keys that match the filter
			for key, value := range data {
				if e.matchesFilter(key, filter.Pattern) {
					result[key] = value
				}
			}
		}
	}

	return result
}

// matchesFilter checks if a key matches a filter pattern using regular expressions
func (e *Extractor) matchesFilter(key, pattern string) bool {
	// Compile the regular expression pattern
	regex, err := regexp.Compile(pattern)
	if err != nil {
		// If pattern is invalid regex, fall back to exact string match
		return pattern == key
	}

	// Use regex to match the key
	return regex.MatchString(key)
}

// UpdateConfig updates the extractor configuration
func (e *Extractor) UpdateConfig(cfg config.ExtractConfig) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.config = cfg
	e.httpClient.Timeout = cfg.Timeout
	e.macroSubstituter = utils.NewMacroSubstituter(cfg.StartTime, cfg.EndTime)
}

// writeDebugOutput writes extraction results to debug file
func (e *Extractor) writeDebugOutput(results []*Result) error {
	// Create debug directory if it doesn't exist
	debugDir := filepath.Dir(e.config.Debug.Path)
	if err := os.MkdirAll(debugDir, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	// Create debug output with timestamp
	debugData := map[string]interface{}{
		"timestamp":     time.Now().Format(time.RFC3339),
		"pipeline":      "extract",
		"results_count": len(results),
		"results":       results,
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(debugData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal debug data: %w", err)
	}

	// Generate filename with timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_extract_%s.json", filepath.Base(e.config.Debug.Path), timestamp)
	fullPath := filepath.Join(debugDir, filename)

	// Write to file
	if err := os.WriteFile(fullPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write debug file: %w", err)
	}

	fmt.Printf("Debug output written to: %s\n", fullPath)
	return nil
}
