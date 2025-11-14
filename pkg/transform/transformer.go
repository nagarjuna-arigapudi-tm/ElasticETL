package transform

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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
		var transformed *TransformedResult
		var err error

		// Check if input format is CSV and CSV data is available
		if t.config.Input.Format == "csv" && len(result.CSVData) > 0 {
			transformed, err = t.transformSingleCSV(result)
		} else {
			transformed, err = t.transformSingle(result)
		}

		if err != nil {
			return nil, fmt.Errorf("failed to transform result from %s: %w", result.Source, err)
		}
		transformedResults = append(transformedResults, transformed)
	}

	// Convert to CSV format if requested (only for JSON input)
	if t.config.OutputFormat == "csv" && t.config.Input.Format != "csv" {
		if err := t.convertToCSV(transformedResults); err != nil {
			return nil, fmt.Errorf("failed to convert to CSV: %w", err)
		}
	}

	// Store results if not stateless
	if !t.config.Stateless {
		t.storePreviousResults(transformedResults)
	}

	// Debug output after transform phase if any debug option is enabled
	if t.shouldWriteDebugOutput() {
		if err := t.writeDebugOutput(results, transformedResults); err != nil {
			fmt.Printf("Failed to write transform debug output: %v\n", err)
		}
	}

	return transformedResults, nil
}

// transformSingle transforms a single result
func (t *Transformer) transformSingle(result *extract.Result) (*TransformedResult, error) {
	transformedData := make(map[string]interface{})

	// Copy original data, optionally dropping null values
	for key, value := range result.Data {
		if t.config.DropNullValues && value == nil {
			// Skip null values if drop_null_values is enabled
			continue
		}
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

// transformSingleCSV transforms a single result with CSV data
func (t *Transformer) transformSingleCSV(result *extract.Result) (*TransformedResult, error) {
	if len(result.CSVData) == 0 {
		return &TransformedResult{
			Result:  result,
			CSVData: [][]string{},
		}, nil
	}

	var csvHeaders []string
	var csvData [][]string
	startRowIndex := 0

	// Handle header extraction if configured
	if t.config.Input.Header && len(result.CSVData) > 0 {
		csvHeaders = make([]string, len(result.CSVData[0]))
		copy(csvHeaders, result.CSVData[0])
		startRowIndex = 1
	}

	// Process each row starting from the appropriate index
	for i := startRowIndex; i < len(result.CSVData); i++ {
		row := result.CSVData[i]

		// Drop entire row if any element is null and drop_null_values is true
		if t.config.DropNullValues {
			hasNull := false
			for _, cell := range row {
				if cell == "" || cell == "null" || cell == "NULL" {
					hasNull = true
					break
				}
			}
			if hasNull {
				continue // Skip this row
			}
		}

		// Apply conversion functions using column index
		transformedRow := make([]string, len(row))
		copy(transformedRow, row)

		for _, convFunc := range t.config.ConversionFunctions {
			if convFunc.FieldIndex >= 0 && convFunc.FieldIndex < len(transformedRow) {
				originalValue := transformedRow[convFunc.FieldIndex]
				convertedValue, err := t.applyCSVConversionFunction(originalValue, convFunc)
				if err != nil {
					return nil, fmt.Errorf("conversion function failed for column %d: %w", convFunc.FieldIndex, err)
				}
				transformedRow[convFunc.FieldIndex] = convertedValue
			}
		}

		csvData = append(csvData, transformedRow)
	}

	return &TransformedResult{
		Result:     result,
		CSVData:    csvData,
		CSVHeaders: csvHeaders,
	}, nil
}

// applyCSVConversionFunction applies conversion function to a CSV cell value
func (t *Transformer) applyCSVConversionFunction(value string, convFunc config.ConversionFunctionConfig) (string, error) {
	switch convFunc.Function {
	case "convert_type":
		converted, err := t.convertType(value, convFunc.FromType, convFunc.ToType)
		if err != nil {
			return value, err
		}
		return t.formatValue(converted), nil

	case "convert_to_kb":
		converted, err := t.convertToKB(value, convFunc.FromUnit)
		if err != nil {
			return value, err
		}
		return t.formatValue(converted), nil

	case "convert_to_mb":
		converted, err := t.convertToMB(value, convFunc.FromUnit)
		if err != nil {
			return value, err
		}
		return t.formatValue(converted), nil

	case "convert_to_gb":
		converted, err := t.convertToGB(value, convFunc.FromUnit)
		if err != nil {
			return value, err
		}
		return t.formatValue(converted), nil

	default:
		return value, fmt.Errorf("unknown conversion function: %s", convFunc.Function)
	}
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

	// Use 'bytes' as default unit if not specified
	if fromUnit == "" {
		fromUnit = "bytes"
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

	// Use 'bytes' as default unit if not specified
	if fromUnit == "" {
		fromUnit = "bytes"
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

	// Use 'bytes' as default unit if not specified
	if fromUnit == "" {
		fromUnit = "bytes"
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
// Returns keys sorted by depth (least depth first), with "key" (case insensitive) prioritized within same depth
func (t *Transformer) analyzeUniqueKeys(results []*TransformedResult) []string {
	// Collect all flattened keys from all results
	allKeys := make(map[string]bool)
	for _, result := range results {
		for key := range result.TransformedData {
			allKeys[key] = true
		}
	}

	// Group keys by depth level and remove array indices
	uniqueKeySet := make(map[string]bool)
	keyDepthMap := make(map[string]int)

	for key := range allKeys {
		uniqueKey := t.removeArrayIndices(key)
		depth := t.calculateKeyDepth(uniqueKey)
		uniqueKeySet[uniqueKey] = true
		keyDepthMap[uniqueKey] = depth
	}

	// Convert to slice for sorting
	var uniqueKeys []string
	for key := range uniqueKeySet {
		uniqueKeys = append(uniqueKeys, key)
	}

	// Sort by depth first (least depth first), then by "key" priority within same depth
	sort.Slice(uniqueKeys, func(i, j int) bool {
		keyI, keyJ := uniqueKeys[i], uniqueKeys[j]
		depthI, depthJ := keyDepthMap[keyI], keyDepthMap[keyJ]

		// First sort by depth (least depth first)
		if depthI != depthJ {
			return depthI < depthJ
		}

		// Within same depth, prioritize "key" (case insensitive)
		keyILower := strings.ToLower(keyI)
		keyJLower := strings.ToLower(keyJ)

		// Check if key name is "key" (case insensitive) - check both full name and suffix
		isKeyI := keyILower == "key" || strings.HasSuffix(keyILower, ".key")
		isKeyJ := keyJLower == "key" || strings.HasSuffix(keyJLower, ".key")

		if isKeyI && !isKeyJ {
			return true
		}
		if !isKeyI && isKeyJ {
			return false
		}

		// If both or neither are "key", sort by length (shorter first)
		if len(keyI) != len(keyJ) {
			return len(keyI) < len(keyJ)
		}

		// Finally, sort alphabetically
		return keyI < keyJ
	})

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

// extractLeftmostIndex extracts the leftmost array index from a flattened key
func (t *Transformer) extractLeftmostIndex(key string) int {
	// Find the first array index in the key
	re := regexp.MustCompile(`\[(\d+)\]`)
	matches := re.FindStringSubmatch(key)

	if len(matches) > 1 {
		if index, err := strconv.Atoi(matches[1]); err == nil {
			return index
		}
	}

	// Return -1 if no array index found (non-array keys will be sorted first)
	return -1
}

// splitKeyIntoParts splits a flattened key into hierarchical parts
func (t *Transformer) splitKeyIntoParts(key string) []string {
	// Split by dots to get path components
	parts := strings.Split(key, ".")
	var result []string

	for _, part := range parts {
		// Handle array indices within each part
		if strings.Contains(part, "[") {
			// Split array-indexed parts further
			re := regexp.MustCompile(`([^\[]+)(\[\d+\])`)
			matches := re.FindAllStringSubmatch(part, -1)

			if len(matches) > 0 {
				for _, match := range matches {
					if len(match) >= 3 {
						// Add the field name with its array index
						result = append(result, match[1]+match[2])
					}
				}
			} else {
				result = append(result, part)
			}
		} else {
			result = append(result, part)
		}
	}

	return result
}

// extractIndexAndField extracts the array index and field name from a path part
func (t *Transformer) extractIndexAndField(part string) (int, string) {
	// Check if this part has an array index
	re := regexp.MustCompile(`^([^\[]+)\[(\d+)\]$`)
	matches := re.FindStringSubmatch(part)

	if len(matches) >= 3 {
		fieldName := matches[1]
		if index, err := strconv.Atoi(matches[2]); err == nil {
			return index, fieldName
		}
	}

	// No array index, return -1 and the field name
	return -1, part
}

// generateCSVRows generates CSV rows from flattened data based on unique keys
// Uses dynamic state tracking to create CSV rows based on depth changes
func (t *Transformer) generateCSVRows(data map[string]interface{}, uniqueKeys []string) [][]string {
	if len(uniqueKeys) == 0 {
		return [][]string{}
	}

	// Create sorted list of flattened data keys
	var sortedKeys []string
	for key := range data {
		sortedKeys = append(sortedKeys, key)
	}

	// Sort flattened data according to hierarchical structure
	sort.Slice(sortedKeys, func(i, j int) bool {
		keyI, keyJ := sortedKeys[i], sortedKeys[j]

		// Split keys into path components for hierarchical comparison
		partsI := t.splitKeyIntoParts(keyI)
		partsJ := t.splitKeyIntoParts(keyJ)

		// Compare path components level by level
		minLen := len(partsI)
		if len(partsJ) < minLen {
			minLen = len(partsJ)
		}

		for level := 0; level < minLen; level++ {
			partI := partsI[level]
			partJ := partsJ[level]

			// Extract index and field name from each part
			indexI, fieldI := t.extractIndexAndField(partI)
			indexJ, fieldJ := t.extractIndexAndField(partJ)

			// Compare indices first
			if indexI != indexJ {
				return indexI < indexJ
			}

			// Within same index, prioritize "key" field
			isKeyI := strings.ToLower(fieldI) == "key"
			isKeyJ := strings.ToLower(fieldJ) == "key"

			if isKeyI && !isKeyJ {
				return true
			}
			if !isKeyI && isKeyJ {
				return false
			}

			// Compare field names alphabetically
			if fieldI != fieldJ {
				return fieldI < fieldJ
			}
		}

		// If all compared levels are equal, shorter path comes first
		return len(partsI) < len(partsJ)
	})

	// Initialize state tracking maps
	valueMap := make(map[string]string, len(uniqueKeys))
	boolMap := make(map[string]bool, len(uniqueKeys))
	depthMap := make(map[string]int, len(uniqueKeys))

	// Initialize maps with unique keys
	for _, uniqueKey := range uniqueKeys {
		valueMap[uniqueKey] = ""
		boolMap[uniqueKey] = false
		depthMap[uniqueKey] = t.calculateKeyDepth(uniqueKey)
	}

	var csvRows [][]string
	currentDepth := 0

	// Process sorted flattened data
	for _, key := range sortedKeys {
		uniqueKey := t.removeArrayIndices(key)
		keyDepth := depthMap[uniqueKey]
		value := t.formatValue(data[key])

		// Check if this unique key exists in our tracking
		if _, exists := depthMap[uniqueKey]; !exists {
			continue
		}

		// Handle depth changes
		if keyDepth < currentDepth {
			// Depth decreased - replace value and reset deeper levels
			valueMap[uniqueKey] = value
			boolMap[uniqueKey] = true
			currentDepth = keyDepth

			// Reset all keys with same or greater depth
			for uk := range depthMap {
				if depthMap[uk] >= keyDepth && uk != uniqueKey {
					valueMap[uk] = ""
					boolMap[uk] = false
				}
			}
		} else {
			// Normal processing - store value and set boolean
			valueMap[uniqueKey] = value
			boolMap[uniqueKey] = true
			if keyDepth > currentDepth {
				currentDepth = keyDepth
			}
		}

		// Check if all booleans are true - create CSV row
		allTrue := true
		for _, val := range boolMap {
			if !val {
				allTrue = false
				break
			}
		}

		if allTrue {
			// Create CSV row
			row := make([]string, len(uniqueKeys))
			for i, uniqueKey := range uniqueKeys {
				row[i] = valueMap[uniqueKey]
			}
			csvRows = append(csvRows, row)

			// Reset entries at maximum depth
			maxDepth := 0
			for _, depth := range depthMap {
				if depth > maxDepth {
					maxDepth = depth
				}
			}

			// Reset values and booleans for maximum depth keys
			for uk := range depthMap {
				if depthMap[uk] == maxDepth {
					valueMap[uk] = ""
					boolMap[uk] = false
				}
			}
		}
	}

	return csvRows
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

// shouldWriteDebugOutput checks if any debug output should be written
func (t *Transformer) shouldWriteDebugOutput() bool {
	return t.config.Debug.Input || t.config.Debug.TransformedOutput || t.config.Debug.FinalOutput
}

// writeDebugOutput writes transformation debug information to file
func (t *Transformer) writeDebugOutput(inputResults []*extract.Result, transformedResults []*TransformedResult) error {
	// Get debug path, default to "debug" if not specified
	debugPath := t.config.Debug.Path
	if debugPath == "" {
		debugPath = "debug"
	}

	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	// Create debug output with timestamp
	debugData := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"stage":     "transform",
	}

	// Add debug information based on configuration
	if t.config.Debug.Input {
		// Include input data based on input format
		inputData := make([]map[string]interface{}, 0, len(inputResults))
		for _, result := range inputResults {
			input := map[string]interface{}{
				"source":    result.Source,
				"timestamp": result.Timestamp,
			}

			if t.config.Input.Format == "csv" {
				input["csv_data"] = result.CSVData
			} else {
				input["data"] = result.Data
			}

			inputData = append(inputData, input)
		}
		debugData["input"] = inputData
	}

	if t.config.Debug.TransformedOutput {
		// Include transformed data (after null handling and conversions, before format change)
		transformedData := make([]map[string]interface{}, 0, len(transformedResults))
		for _, result := range transformedResults {
			transformed := map[string]interface{}{
				"source":    result.Source,
				"timestamp": result.Timestamp,
			}

			if t.config.Input.Format == "csv" {
				// For CSV input, show the processed CSV data before final output
				transformed["transformed_csv_data"] = result.CSVData
				if len(result.CSVHeaders) > 0 {
					transformed["csv_headers"] = result.CSVHeaders
				}
			} else {
				// For JSON input, show the transformed JSON data
				transformed["transformed_data"] = result.TransformedData
			}

			transformedData = append(transformedData, transformed)
		}
		debugData["transformed_output"] = transformedData
	}

	if t.config.Debug.FinalOutput {
		// Include final output data (what gets passed to load stage)
		finalOutput := make([]map[string]interface{}, 0, len(transformedResults))
		for _, result := range transformedResults {
			output := map[string]interface{}{
				"source":    result.Source,
				"timestamp": result.Timestamp,
			}

			// Final output is always CSV format for now
			output["csv_data"] = result.CSVData
			if len(result.CSVHeaders) > 0 {
				output["csv_headers"] = result.CSVHeaders
			}

			finalOutput = append(finalOutput, output)
		}
		debugData["final_output"] = finalOutput
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(debugData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal debug data: %w", err)
	}

	// Generate filename with pipeline name and timestamp
	timestamp := time.Now().Format("20060102_150405")
	// Note: Pipeline name will be passed from pipeline level, for now use "unknown"
	filename := fmt.Sprintf("unknown_transform_%s.json", timestamp)
	fullPath := filepath.Join(debugPath, filename)

	// Write to file
	if err := os.WriteFile(fullPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write debug file: %w", err)
	}

	fmt.Printf("Transform debug output written to: %s\n", fullPath)
	return nil
}

// WriteDebugOutputWithPipelineName writes transformation debug information to file with pipeline name
func (t *Transformer) WriteDebugOutputWithPipelineName(inputResults []*extract.Result, transformedResults []*TransformedResult, pipelineName string) error {
	// Get debug path, default to "debug" if not specified
	debugPath := t.config.Debug.Path
	if debugPath == "" {
		debugPath = "debug"
	}

	// Create debug directory if it doesn't exist
	if err := os.MkdirAll(debugPath, 0755); err != nil {
		return fmt.Errorf("failed to create debug directory: %w", err)
	}

	// Create debug output with timestamp
	debugData := map[string]interface{}{
		"timestamp":     time.Now().Format(time.RFC3339),
		"stage":         "transform",
		"pipeline_name": pipelineName,
	}

	// Add debug information based on configuration
	if t.config.Debug.Input {
		// Include input data based on input format
		inputData := make([]map[string]interface{}, 0, len(inputResults))
		for _, result := range inputResults {
			input := map[string]interface{}{
				"source":    result.Source,
				"timestamp": result.Timestamp,
			}

			if t.config.Input.Format == "csv" {
				input["csv_data"] = result.CSVData
			} else {
				input["data"] = result.Data
			}

			inputData = append(inputData, input)
		}
		debugData["input"] = inputData
	}

	if t.config.Debug.TransformedOutput {
		// Include transformed data (after null handling and conversions, before format change)
		transformedData := make([]map[string]interface{}, 0, len(transformedResults))
		for _, result := range transformedResults {
			transformed := map[string]interface{}{
				"source":    result.Source,
				"timestamp": result.Timestamp,
			}

			if t.config.Input.Format == "csv" {
				// For CSV input, show the processed CSV data before final output
				transformed["transformed_csv_data"] = result.CSVData
				if len(result.CSVHeaders) > 0 {
					transformed["csv_headers"] = result.CSVHeaders
				}
			} else {
				// For JSON input, show the transformed JSON data
				transformed["transformed_data"] = result.TransformedData
			}

			transformedData = append(transformedData, transformed)
		}
		debugData["transformed_output"] = transformedData
	}

	if t.config.Debug.FinalOutput {
		// Include final output data (what gets passed to load stage)
		finalOutput := make([]map[string]interface{}, 0, len(transformedResults))
		for _, result := range transformedResults {
			output := map[string]interface{}{
				"source":    result.Source,
				"timestamp": result.Timestamp,
			}

			// Final output is always CSV format for now
			output["csv_data"] = result.CSVData
			if len(result.CSVHeaders) > 0 {
				output["csv_headers"] = result.CSVHeaders
			}

			finalOutput = append(finalOutput, output)
		}
		debugData["final_output"] = finalOutput
	}

	// Marshal to JSON
	jsonData, err := json.MarshalIndent(debugData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal debug data: %w", err)
	}

	// Generate filename with pipeline name and timestamp
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("%s_transform_%s.json", pipelineName, timestamp)
	fullPath := filepath.Join(debugPath, filename)

	// Write to file
	if err := os.WriteFile(fullPath, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to write debug file: %w", err)
	}

	fmt.Printf("Transform debug output written to: %s\n", fullPath)
	return nil
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
