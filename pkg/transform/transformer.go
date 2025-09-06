package transform

import (
	"fmt"
	"reflect"
	"sort"
	"strconv"
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

// applyConversionFunction applies a conversion function to a field
func (t *Transformer) applyConversionFunction(data map[string]interface{}, convFunc config.ConversionFunctionConfig) error {
	value, exists := data[convFunc.Field]
	if !exists {
		return nil // Field doesn't exist, skip
	}

	switch convFunc.Function {
	case "convert_type":
		converted, err := t.convertType(value, convFunc.FromType, convFunc.ToType)
		if err != nil {
			return err
		}
		data[convFunc.Field] = converted

	case "convert_to_kb":
		converted, err := t.convertToKB(value, convFunc.FromUnit)
		if err != nil {
			return err
		}
		data[convFunc.Field] = converted

	case "convert_to_mb":
		converted, err := t.convertToMB(value, convFunc.FromUnit)
		if err != nil {
			return err
		}
		data[convFunc.Field] = converted

	case "convert_to_gb":
		converted, err := t.convertToGB(value, convFunc.FromUnit)
		if err != nil {
			return err
		}
		data[convFunc.Field] = converted

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

// convertToCSV converts flattened data to CSV format
func (t *Transformer) convertToCSV(results []*TransformedResult) error {
	if len(results) == 0 {
		return nil
	}

	// Collect all unique column names from all results
	columnSet := make(map[string]bool)
	for _, result := range results {
		for key := range result.TransformedData {
			columnSet[key] = true
		}
	}

	// Convert to sorted slice for consistent column order
	var columns []string
	for col := range columnSet {
		columns = append(columns, col)
	}
	sort.Strings(columns)

	// Set headers for all results
	for _, result := range results {
		result.CSVHeaders = columns
	}

	// Convert each result to CSV rows
	for _, result := range results {
		rows := t.flattenToCSVRows(result.TransformedData, columns)
		result.CSVData = rows
	}

	return nil
}

// flattenToCSVRows converts flattened data to CSV rows with proper nested array handling
func (t *Transformer) flattenToCSVRows(data map[string]interface{}, columns []string) [][]string {
	// Find all nested array structures and calculate total rows needed
	nestedArrays := t.findNestedArrays(data)

	if len(nestedArrays) == 0 {
		// No nested arrays, create single row
		row := make([]string, len(columns))
		for colIdx, column := range columns {
			if value, exists := data[column]; exists {
				row[colIdx] = t.formatValue(value)
			}
		}
		return [][]string{row}
	}

	// Calculate total rows needed by finding the maximum array length
	totalRows := t.calculateTotalRows(data, nestedArrays)
	rows := make([][]string, totalRows)

	// Generate rows by expanding nested arrays
	rowIndex := 0
	t.expandNestedArrays(data, nestedArrays, columns, &rows, &rowIndex, make(map[string]interface{}), 0)

	return rows[:rowIndex] // Return only the rows that were actually filled
}

// findNestedArrays identifies nested array structures in flattened data
func (t *Transformer) findNestedArrays(data map[string]interface{}) map[string][]interface{} {
	nestedArrays := make(map[string][]interface{})

	for key, value := range data {
		if arr, ok := value.([]interface{}); ok {
			// Check if this is a nested array (contains objects)
			if len(arr) > 0 {
				if _, isObject := arr[0].(map[string]interface{}); isObject {
					nestedArrays[key] = arr
				}
			}
		}
	}

	return nestedArrays
}

// calculateTotalRows calculates the total number of CSV rows needed
func (t *Transformer) calculateTotalRows(data map[string]interface{}, nestedArrays map[string][]interface{}) int {
	if len(nestedArrays) == 0 {
		return 1
	}

	// For nested structures, multiply the lengths of all nested arrays
	totalRows := 1
	for _, arr := range nestedArrays {
		if len(arr) > 0 {
			totalRows *= len(arr)
		}
	}

	return totalRows
}

// expandNestedArrays recursively expands nested arrays into CSV rows
func (t *Transformer) expandNestedArrays(data map[string]interface{}, nestedArrays map[string][]interface{},
	columns []string, rows *[][]string, rowIndex *int, currentRow map[string]interface{}, arrayIndex int) {

	// Get array keys in sorted order for consistent processing
	var arrayKeys []string
	for key := range nestedArrays {
		arrayKeys = append(arrayKeys, key)
	}
	sort.Strings(arrayKeys)

	if arrayIndex >= len(arrayKeys) {
		// Base case: create a CSV row with current values
		if *rowIndex < len(*rows) {
			(*rows)[*rowIndex] = make([]string, len(columns))

			// Fill the row with values
			for colIdx, column := range columns {
				if value, exists := currentRow[column]; exists {
					(*rows)[*rowIndex][colIdx] = t.formatValue(value)
				} else if value, exists := data[column]; exists {
					// Use original data if not in current row (non-array values)
					(*rows)[*rowIndex][colIdx] = t.formatValue(value)
				}
			}
			*rowIndex++
		}
		return
	}

	// Process current array level
	currentArrayKey := arrayKeys[arrayIndex]
	currentArray := nestedArrays[currentArrayKey]

	for _, arrayItem := range currentArray {
		// Create a new row context with current array item values
		newRow := make(map[string]interface{})
		for k, v := range currentRow {
			newRow[k] = v
		}

		// Add values from current array item
		if itemMap, ok := arrayItem.(map[string]interface{}); ok {
			for key, value := range itemMap {
				// Create flattened key name (e.g., "hosts.key", "hosts.cpu_usage")
				flattenedKey := currentArrayKey + "." + key
				newRow[flattenedKey] = value
			}
		}

		// Recursively process next array level
		t.expandNestedArrays(data, nestedArrays, columns, rows, rowIndex, newRow, arrayIndex+1)
	}
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
	case float64, float32:
		return fmt.Sprintf("%g", v)
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
