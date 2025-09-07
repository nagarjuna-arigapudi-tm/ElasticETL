package transform

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"elasticetl/pkg/config"
	"elasticetl/pkg/extract"
)

// TransformedResult represents transformed data
type TransformedResult struct {
	*extract.Result
	TransformedData map[string]interface{} `json:"transformed_data"`
	CSVData         [][]string             `json:"csv_data,omitempty"`    // CSV format data
	CSVHeaders      []string               `json:"csv_headers,omitempty"` // CSV column headers
}

// Transformer handles data transformation
type Transformer struct {
	config          config.TransformConfig
	previousResults [][]*TransformedResult
	mutex           sync.RWMutex
}

// NewTransformer creates a new transformer
func NewTransformer(cfg config.TransformConfig) *Transformer {
	return &Transformer{
		config:          cfg,
		previousResults: make([][]*TransformedResult, 0, cfg.PreviousResultsSets),
	}
}

// Transform performs data transformation
func (t *Transformer) Transform(results []*extract.Result) ([]*TransformedResult, error) {
	var transformedResults []*TransformedResult

	for _, result := range results {
		transformed, err := t.transformSingle(result)
		if err != nil {
			return nil, fmt.Errorf("failed to transform result from %s: %w", result.Source, err)
		}
		transformedResults = append(transformedResults, transformed)
	}

	// Convert to CSV format if requested
	if t.config.OutputFormat == "csv" {
		if err := t.convertToCSV(transformedResults); err != nil {
			return nil, fmt.Errorf("failed to convert to CSV: %w", err)
		}
	}

	// Store results if not stateless
	if !t.config.Stateless {
		t.storePreviousResults(transformedResults)
	}

	return transformedResults, nil
}

// transformSingle transforms a single result
func (t *Transformer) transformSingle(result *extract.Result) (*TransformedResult, error) {
	transformedData := make(map[string]interface{})

	// Copy original data
	for key, value := range result.Data {
		transformedData[key] = value
	}

	// Apply null/zero substitution
	if t.config.SubstituteZerosForNull {
		t.substituteZerosForNull(transformedData)
	}

	// Apply conversion functions
	for _, convFunc := range t.config.ConversionFunctions {
		if err := t.applyConversionFunction(transformedData, convFunc); err != nil {
			return nil, fmt.Errorf("conversion function failed for field %s: %w", convFunc.Field, err)
		}
	}

	return &TransformedResult{
		Result:          result,
		TransformedData: transformedData,
	}, nil
}

// substituteZerosForNull replaces null/nil values with zeros
func (t *Transformer) substituteZerosForNull(data map[string]interface{}) {
	for key, value := range data {
		if value == nil {
			// Determine appropriate zero value based on context
			data[key] = 0
		} else if reflect.ValueOf(value).Kind() == reflect.Map {
			if nestedMap, ok := value.(map[string]interface{}); ok {
				t.substituteZerosForNull(nestedMap)
			}
		}
	}
}

// applyConversionFunction applies a conversion function to fields matching regex pattern
func (t *Transformer) applyConversionFunction(data map[string]interface{}, convFunc config.ConversionFunctionConfig) error {
	// Compile regex pattern for field matching
	regex, err := regexp.Compile(convFunc.Field)
	if err != nil {
		// If regex is invalid, try exact match as fallback
		value, exists := data[convFunc.Field]
		if !exists {
			return nil // Field doesn't exist, skip
		}
		return t.applyConversionToValue(data, convFunc.Field, value, convFunc)
	}

	// Apply conversion to all matching fields
	matchedAny := false
	for key, value := range data {
		if regex.MatchString(key) {
			matchedAny = true
			if err := t.applyConversionToValue(data, key, value, convFunc); err != nil {
				return fmt.Errorf("conversion failed for field %s: %w", key, err)
			}
		}
	}

	if !matchedAny {
		return nil // No fields matched, skip
	}

	return nil
}

// applyConversionToValue applies conversion function to a specific field value
func (t *Transformer) applyConversionToValue(data map[string]interface{}, fieldKey string, value interface{}, convFunc config.ConversionFunctionConfig) error {
	switch convFunc.Function {
	case "convert_type":
		converted, err := t.convertType(value, convFunc.FromType, convFunc.ToType)
		if err != nil {
			return err
		}
		data[fieldKey] = converted

	case "convert_to_kb":
		converted, err := t.convertToKB(value, convFunc.FromUnit)
		if err != nil {
			return err
		}
		data[fieldKey] = converted

	case "convert_to_mb":
		converted, err := t.convertToMB(value, convFunc.FromUnit)
		if err != nil {
			return err
		}
		data[fieldKey] = converted

	case "convert_to_gb":
		converted, err := t.convertToGB(value, convFunc.FromUnit)
		if err != nil {
			return err
		}
		data[fieldKey] = converted

	default:
		return fmt.Errorf("unknown conversion function: %s", convFunc.Function)
	}

	return nil
}

// convertType converts a value from one type to another
func (t *Transformer) convertType(value interface{}, fromType, toType string) (interface{}, error) {
	switch toType {
	case "string":
		return fmt.Sprintf("%v", value), nil
	case "int":
		return t.toInt(value)
	case "float":
		return t.toFloat(value)
	case "bool":
		return t.toBool(value)
	default:
		return nil, fmt.Errorf("unsupported target type: %s", toType)
	}
}

// convertToKB converts a value to kilobytes
func (t *Transformer) convertToKB(value interface{}, fromUnit string) (float64, error) {
	numValue, err := t.toFloat(value)
	if err != nil {
		return 0, err
	}

	switch fromUnit {
	case "bytes", "b":
		return numValue / 1024, nil
	case "kb":
		return numValue, nil
	case "mb":
		return numValue * 1024, nil
	case "gb":
		return numValue * 1024 * 1024, nil
	default:
		return 0, fmt.Errorf("unsupported unit: %s", fromUnit)
	}
}

// convertToMB converts a value to megabytes
func (t *Transformer) convertToMB(value interface{}, fromUnit string) (float64, error) {
	numValue, err := t.toFloat(value)
	if err != nil {
		return 0, err
	}

	switch fromUnit {
	case "bytes", "b":
		return numValue / (1024 * 1024), nil
	case "kb":
		return numValue / 1024, nil
	case "mb":
		return numValue, nil
	case "gb":
		return numValue * 1024, nil
	default:
		return 0, fmt.Errorf("unsupported unit: %s", fromUnit)
	}
}

// convertToGB converts a value to gigabytes
func (t *Transformer) convertToGB(value interface{}, fromUnit string) (float64, error) {
	numValue, err := t.toFloat(value)
	if err != nil {
		return 0, err
	}

	switch fromUnit {
	case "bytes", "b":
		return numValue / (1024 * 1024 * 1024), nil
	case "kb":
		return numValue / (1024 * 1024), nil
	case "mb":
		return numValue / 1024, nil
	case "gb":
		return numValue, nil
	default:
		return 0, fmt.Errorf("unsupported unit: %s", fromUnit)
	}
}

// Helper functions for type conversion
func (t *Transformer) toInt(value interface{}) (int64, error) {
	switch v := value.(type) {
	case int:
		return int64(v), nil
	case int64:
		return v, nil
	case float64:
		return int64(v), nil
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", value)
	}
}

func (t *Transformer) toFloat(value interface{}) (float64, error) {
	switch v := value.(type) {
	case int:
		return float64(v), nil
	case int64:
		return float64(v), nil
	case float64:
		return v, nil
	case string:
		return strconv.ParseFloat(v, 64)
	default:
		return 0, fmt.Errorf("cannot convert %T to float", value)
	}
}

func (t *Transformer) toBool(value interface{}) (bool, error) {
	switch v := value.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(v)
	case int, int64:
		return v != 0, nil
	case float64:
		return v != 0, nil
	default:
		return false, fmt.Errorf("cannot convert %T to bool", value)
	}
}

// storePreviousResults stores results for non-stateless transformations
func (t *Transformer) storePreviousResults(results []*TransformedResult) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	// Add current results
	t.previousResults = append(t.previousResults, results)

	// Keep only the configured number of previous result sets
	if len(t.previousResults) > t.config.PreviousResultsSets {
		t.previousResults = t.previousResults[len(t.previousResults)-t.config.PreviousResultsSets:]
	}
}

// GetPreviousResults returns previous transformation results
func (t *Transformer) GetPreviousResults() [][]*TransformedResult {
	t.mutex.RLock()
	defer t.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make([][]*TransformedResult, len(t.previousResults))
	copy(result, t.previousResults)
	return result
}

// convertToCSV converts flattened data to CSV format using depth-based unique key analysis
func (t *Transformer) convertToCSV(results []*TransformedResult) error {
	if len(results) == 0 {
		return nil
	}

	// Analyze all flattened keys to determine unique column names
	uniqueKeys := t.analyzeUniqueKeys(results)

	// Set headers for all results
	for _, result := range results {
		result.CSVHeaders = uniqueKeys
	}

	// Convert each result to CSV rows
	for _, result := range results {
		rows := t.generateCSVRows(result.TransformedData, uniqueKeys)
		result.CSVData = rows
	}

	return nil
}

// analyzeUniqueKeys analyzes flattened JSON keys by depth levels to determine unique column names
func (t *Transformer) analyzeUniqueKeys(results []*TransformedResult) []string {
	// Collect all flattened keys from all results
	allKeys := make(map[string]bool)
	for _, result := range results {
		for key := range result.TransformedData {
			allKeys[key] = true
		}
	}

	// Group keys by depth level
	keysByDepth := make(map[int][]string)
	maxDepth := 0

	for key := range allKeys {
		depth := t.calculateKeyDepth(key)
		keysByDepth[depth] = append(keysByDepth[depth], key)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	// Process each depth level to extract unique keys
	uniqueKeySet := make(map[string]bool)

	for depth := 1; depth <= maxDepth; depth++ {
		keys := keysByDepth[depth]
		for _, key := range keys {
			uniqueKey := t.removeArrayIndices(key)
			uniqueKeySet[uniqueKey] = true
		}
	}

	// Convert to sorted slice for consistent column order
	var uniqueKeys []string
	for key := range uniqueKeySet {
		uniqueKeys = append(uniqueKeys, key)
	}
	sort.Strings(uniqueKeys)

	return uniqueKeys
}

// calculateKeyDepth calculates the depth level of a flattened key
func (t *Transformer) calculateKeyDepth(key string) int {
	// Count the number of dots and array indices to determine depth
	depth := 1 // Start with 1 for the base level

	// Count dots (each dot represents a level)
	for _, char := range key {
		if char == '.' {
			depth++
		}
	}

	return depth
}

// removeArrayIndices removes array indices from a flattened key to create unique column name
func (t *Transformer) removeArrayIndices(key string) string {
	// Remove array indices like [0], [1], etc.
	re := regexp.MustCompile(`\[\d+\]`)
	return re.ReplaceAllString(key, "")
}

// generateCSVRows generates CSV rows from flattened data based on unique keys
func (t *Transformer) generateCSVRows(data map[string]interface{}, uniqueKeys []string) [][]string {
	// Find all array paths and their combinations
	arrayPaths := t.findArrayPaths(data)

	if len(arrayPaths) == 0 {
		// No arrays, create single row
		row := make([]string, len(uniqueKeys))
		for colIdx, uniqueKey := range uniqueKeys {
			// Find matching key in data
			if value := t.findValueForUniqueKey(data, uniqueKey); value != nil {
				row[colIdx] = t.formatValue(value)
			}
		}
		return [][]string{row}
	}

	// Generate all possible combinations of array indices
	combinations := t.generateArrayCombinations(arrayPaths)

	// Create rows for each combination
	var rows [][]string
	for _, combination := range combinations {
		row := make([]string, len(uniqueKeys))
		for colIdx, uniqueKey := range uniqueKeys {
			value := t.findValueForCombination(data, uniqueKey, combination)
			row[colIdx] = t.formatValue(value)
		}
		rows = append(rows, row)
	}

	return rows
}

// findArrayPaths identifies all array paths in the flattened data
func (t *Transformer) findArrayPaths(data map[string]interface{}) map[string][]int {
	arrayPaths := make(map[string][]int)

	for key := range data {
		// Extract array path and index
		if path, index := t.extractArrayPathAndIndex(key); path != "" {
			if _, exists := arrayPaths[path]; !exists {
				arrayPaths[path] = []int{}
			}
			// Add index if not already present
			found := false
			for _, existingIndex := range arrayPaths[path] {
				if existingIndex == index {
					found = true
					break
				}
			}
			if !found {
				arrayPaths[path] = append(arrayPaths[path], index)
			}
		}
	}

	// Sort indices for each path
	for path := range arrayPaths {
		sort.Ints(arrayPaths[path])
	}

	return arrayPaths
}

// extractArrayPathAndIndex extracts the array path and index from a flattened key
func (t *Transformer) extractArrayPathAndIndex(key string) (string, int) {
	// Find array indices in the key
	re := regexp.MustCompile(`\[(\d+)\]`)
	matches := re.FindAllStringSubmatch(key, -1)

	if len(matches) == 0 {
		return "", -1
	}

	// Get the deepest array path (last array index in the key)
	lastMatch := matches[len(matches)-1]
	index := 0
	if len(lastMatch) > 1 {
		if parsed, err := strconv.Atoi(lastMatch[1]); err == nil {
			index = parsed
		}
	}

	// Extract the path up to the last array index
	lastIndexPos := strings.LastIndex(key, lastMatch[0])
	if lastIndexPos == -1 {
		return "", -1
	}

	path := key[:lastIndexPos]
	return path, index
}

// generateArrayCombinations generates all possible combinations of array indices
func (t *Transformer) generateArrayCombinations(arrayPaths map[string][]int) []map[string]int {
	if len(arrayPaths) == 0 {
		return []map[string]int{{}}
	}

	// Get sorted paths for consistent processing
	var paths []string
	for path := range arrayPaths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	// Generate combinations recursively
	return t.generateCombinationsRecursive(paths, arrayPaths, 0, make(map[string]int))
}

// generateCombinationsRecursive recursively generates array index combinations
func (t *Transformer) generateCombinationsRecursive(paths []string, arrayPaths map[string][]int, pathIndex int, currentCombination map[string]int) []map[string]int {
	if pathIndex >= len(paths) {
		// Base case: copy current combination
		combination := make(map[string]int)
		for k, v := range currentCombination {
			combination[k] = v
		}
		return []map[string]int{combination}
	}

	path := paths[pathIndex]
	indices := arrayPaths[path]

	var allCombinations []map[string]int
	for _, index := range indices {
		currentCombination[path] = index
		combinations := t.generateCombinationsRecursive(paths, arrayPaths, pathIndex+1, currentCombination)
		allCombinations = append(allCombinations, combinations...)
	}

	return allCombinations
}

// findValueForUniqueKey finds the value for a unique key in the flattened data
func (t *Transformer) findValueForUniqueKey(data map[string]interface{}, uniqueKey string) interface{} {
	// Try exact match first
	if value, exists := data[uniqueKey]; exists {
		return value
	}

	// Look for keys that match the unique key pattern (with array indices)
	for key, value := range data {
		if t.removeArrayIndices(key) == uniqueKey {
			return value
		}
	}

	return nil
}

// findValueForCombination finds the value for a unique key with specific array index combination
func (t *Transformer) findValueForCombination(data map[string]interface{}, uniqueKey string, combination map[string]int) interface{} {
	// Try exact match first (for non-array keys)
	if value, exists := data[uniqueKey]; exists {
		return value
	}

	// Build the specific key with array indices from combination
	specificKey := t.buildSpecificKey(uniqueKey, combination)
	if value, exists := data[specificKey]; exists {
		return value
	}

	// Look for any matching key with the right pattern
	for key, value := range data {
		if t.matchesKeyPattern(key, uniqueKey, combination) {
			return value
		}
	}

	return nil
}

// buildSpecificKey builds a specific key with array indices from combination
func (t *Transformer) buildSpecificKey(uniqueKey string, combination map[string]int) string {
	result := uniqueKey

	// Sort paths by length (longest first) to handle nested arrays correctly
	var paths []string
	for path := range combination {
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		return len(paths[i]) > len(paths[j])
	})

	// Replace each array path with its specific index
	for _, path := range paths {
		index := combination[path]
		if strings.HasPrefix(result, path) {
			result = strings.Replace(result, path, fmt.Sprintf("%s[%d]", path, index), 1)
		}
	}

	return result
}

// matchesKeyPattern checks if a key matches the unique key pattern with given combination
func (t *Transformer) matchesKeyPattern(key, uniqueKey string, combination map[string]int) bool {
	// Remove array indices from the key and compare with unique key
	keyWithoutIndices := t.removeArrayIndices(key)
	if keyWithoutIndices != uniqueKey {
		return false
	}

	// Check if the key's array indices match the combination
	for path, expectedIndex := range combination {
		if strings.Contains(key, path) {
			// Extract the actual index from the key for this path
			pattern := regexp.MustCompile(regexp.QuoteMeta(path) + `\[(\d+)\]`)
			matches := pattern.FindStringSubmatch(key)
			if len(matches) > 1 {
				if actualIndex, err := strconv.Atoi(matches[1]); err == nil {
					if actualIndex != expectedIndex {
						return false
					}
				}
			}
		}
	}

	return true
}

// getArraySize returns the size of an array value, or 1 for non-arrays
func (t *Transformer) getArraySize(value interface{}) int {
	switch v := value.(type) {
	case []interface{}:
		return len(v)
	case []string:
		return len(v)
	case []int:
		return len(v)
	case []float64:
		return len(v)
	default:
		return 1
	}
}

// extractColumnValues extracts values for a column, handling arrays and repetition
func (t *Transformer) extractColumnValues(data map[string]interface{}, column string, maxRows int) []interface{} {
	values := make([]interface{}, maxRows)

	if value, exists := data[column]; exists {
		switch v := value.(type) {
		case []interface{}:
			// Array values - each element goes to a different row
			for i, item := range v {
				if i < maxRows {
					values[i] = item
				}
			}
			// Fill remaining rows with empty values
			for i := len(v); i < maxRows; i++ {
				values[i] = ""
			}
		case []string:
			for i, item := range v {
				if i < maxRows {
					values[i] = item
				}
			}
			for i := len(v); i < maxRows; i++ {
				values[i] = ""
			}
		case []int:
			for i, item := range v {
				if i < maxRows {
					values[i] = item
				}
			}
			for i := len(v); i < maxRows; i++ {
				values[i] = ""
			}
		case []float64:
			for i, item := range v {
				if i < maxRows {
					values[i] = item
				}
			}
			for i := len(v); i < maxRows; i++ {
				values[i] = ""
			}
		default:
			// Single value - repeat for all rows
			for i := 0; i < maxRows; i++ {
				values[i] = value
			}
		}
	} else {
		// Column doesn't exist - fill with empty values
		for i := 0; i < maxRows; i++ {
			values[i] = ""
		}
	}

	return values
}

// formatValue converts a value to string for CSV
func (t *Transformer) formatValue(value interface{}) string {
	if value == nil {
		return ""
	}

	switch v := value.(type) {
	case string:
		return v
	case int, int64, int32:
		return fmt.Sprintf("%d", v)
	case float64:
		// Use fixed-point notation to preserve precision and avoid exponential form
		return fmt.Sprintf("%.15f", v)
	case float32:
		// Use fixed-point notation to preserve precision and avoid exponential form
		return fmt.Sprintf("%.7f", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// UpdateConfig updates the transformer configuration
func (t *Transformer) UpdateConfig(cfg config.TransformConfig) {
	t.mutex.Lock()
	defer t.mutex.Unlock()

	t.config = cfg

	// Adjust previous results storage if needed
	if len(t.previousResults) > cfg.PreviousResultsSets {
		t.previousResults = t.previousResults[len(t.previousResults)-cfg.PreviousResultsSets:]
	}
}
