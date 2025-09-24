package utils

import (
	"fmt"
	"reflect"
	"strconv"
)

// SafeString safely converts a value to string, handling both JSON and YAML parsing
func SafeString(value interface{}) (string, bool) {
	if value == nil {
		return "", false
	}

	switch v := value.(type) {
	case string:
		return v, true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case float64:
		// Check if it's actually an integer value
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case float32:
		if v == float32(int32(v)) {
			return strconv.FormatInt(int64(v), 10), true
		}
		return strconv.FormatFloat(float64(v), 'f', -1, 32), true
	case bool:
		return strconv.FormatBool(v), true
	default:
		return fmt.Sprintf("%v", v), true
	}
}

// SafeInt safely converts a value to int, handling both JSON and YAML parsing
func SafeInt(value interface{}) (int, bool) {
	if value == nil {
		return 0, false
	}

	switch v := value.(type) {
	case int:
		return v, true
	case int64:
		return int(v), true
	case int32:
		return int(v), true
	case float64:
		// Check if it's actually an integer value
		if v == float64(int64(v)) {
			return int(v), true
		}
		return 0, false
	case float32:
		if v == float32(int32(v)) {
			return int(v), true
		}
		return 0, false
	case string:
		if i, err := strconv.Atoi(v); err == nil {
			return i, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// SafeFloat64 safely converts a value to float64, handling both JSON and YAML parsing
func SafeFloat64(value interface{}) (float64, bool) {
	if value == nil {
		return 0, false
	}

	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
		return 0, false
	default:
		return 0, false
	}
}

// SafeBool safely converts a value to bool, handling both JSON and YAML parsing
func SafeBool(value interface{}) (bool, bool) {
	if value == nil {
		return false, false
	}

	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		if b, err := strconv.ParseBool(v); err == nil {
			return b, true
		}
		return false, false
	case int:
		return v != 0, true
	case int64:
		return v != 0, true
	case float64:
		return v != 0, true
	default:
		return false, false
	}
}

// SafeMapStringInterface safely converts a value to map[string]interface{}, handling both JSON and YAML parsing
func SafeMapStringInterface(value interface{}) (map[string]interface{}, bool) {
	if value == nil {
		return nil, false
	}

	switch v := value.(type) {
	case map[string]interface{}:
		return v, true
	case map[interface{}]interface{}:
		// YAML sometimes parses maps as map[interface{}]interface{}
		result := make(map[string]interface{})
		for key, val := range v {
			if strKey, ok := SafeString(key); ok {
				result[strKey] = val
			}
		}
		return result, true
	default:
		return nil, false
	}
}

// SafeSliceInterface safely converts a value to []interface{}, handling both JSON and YAML parsing
func SafeSliceInterface(value interface{}) ([]interface{}, bool) {
	if value == nil {
		return nil, false
	}

	switch v := value.(type) {
	case []interface{}:
		return v, true
	default:
		// Check if it's a slice using reflection
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice {
			result := make([]interface{}, rv.Len())
			for i := 0; i < rv.Len(); i++ {
				result[i] = rv.Index(i).Interface()
			}
			return result, true
		}
		return nil, false
	}
}
