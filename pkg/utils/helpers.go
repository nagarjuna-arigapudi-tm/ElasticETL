package utils

import (
	"strings"
)

// GetLastPathSegment returns the last segment of a JSON path
func GetLastPathSegment(jsonPath string) string {
	parts := strings.Split(jsonPath, ".")
	if len(parts) == 0 {
		return jsonPath
	}

	lastPart := parts[len(parts)-1]

	// Remove array indices if present
	if idx := strings.Index(lastPart, "["); idx != -1 {
		lastPart = lastPart[:idx]
	}

	return lastPart
}

// Contains checks if a slice contains a string
func Contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// RemoveEmpty removes empty strings from a slice
func RemoveEmpty(slice []string) []string {
	var result []string
	for _, s := range slice {
		if strings.TrimSpace(s) != "" {
			result = append(result, s)
		}
	}
	return result
}
