package extract

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"elasticetl/pkg/config"
	"elasticetl/pkg/utils"

	"github.com/tidwall/gjson"
)

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

	// Add auth header if provided
	if len(e.config.AuthHeaders) > index && e.config.AuthHeaders[index] != "" {
		req.Header.Set("Authorization", e.config.AuthHeaders[index])
	}

	// Add additional headers if provided
	if len(e.config.AdditionalHeaders) > index && len(e.config.AdditionalHeaders[index]) > 0 {
		for _, header := range e.config.AdditionalHeaders[index] {
			// Each header should be in format "Key: Value"
			if len(header) > 0 {
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
		body, _ := ioutil.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Read response
	body, err := ioutil.ReadAll(resp.Body)
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

// extractDataFromResponse extracts data from Elasticsearch response using configured JSON paths
func (e *Extractor) extractDataFromResponse(responseBody []byte) (map[string]interface{}, error) {
	if len(e.config.JSONPaths) == 0 {
		// If no JSON paths specified, return the entire response
		var data map[string]interface{}
		if err := json.Unmarshal(responseBody, &data); err != nil {
			return nil, fmt.Errorf("failed to unmarshal response: %w", err)
		}
		return data, nil
	}

	extractedData := make(map[string]interface{})
	responseStr := string(responseBody)

	for _, jsonPath := range e.config.JSONPaths {
		result := gjson.Get(responseStr, jsonPath)
		if result.Exists() {
			// Use the last part of the JSON path as the key
			key := utils.GetLastPathSegment(jsonPath)

			// Convert gjson.Result to appropriate Go type
			switch result.Type {
			case gjson.String:
				extractedData[key] = result.String()
			case gjson.Number:
				extractedData[key] = result.Num
			case gjson.True, gjson.False:
				extractedData[key] = result.Bool()
			case gjson.JSON:
				var value interface{}
				if err := json.Unmarshal([]byte(result.Raw), &value); err == nil {
					extractedData[key] = value
				} else {
					extractedData[key] = result.Raw
				}
			default:
				extractedData[key] = result.Value()
			}
		}
	}

	return extractedData, nil
}

// UpdateConfig updates the extractor configuration
func (e *Extractor) UpdateConfig(cfg config.ExtractConfig) {
	e.mutex.Lock()
	defer e.mutex.Unlock()

	e.config = cfg
	e.httpClient.Timeout = cfg.Timeout
	e.macroSubstituter = utils.NewMacroSubstituter(cfg.StartTime, cfg.EndTime)
}
