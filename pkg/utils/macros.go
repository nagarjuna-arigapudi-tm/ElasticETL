package utils

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// MacroSubstituter handles macro substitution in queries
type MacroSubstituter struct {
	startTime string
	endTime   string
}

// NewMacroSubstituter creates a new macro substituter
func NewMacroSubstituter(startTime, endTime string) *MacroSubstituter {
	return &MacroSubstituter{
		startTime: startTime,
		endTime:   endTime,
	}
}

// SubstituteQuery substitutes macros in the query string
func (m *MacroSubstituter) SubstituteQuery(query, clusterName string) (string, error) {
	result := query

	// Substitute __CLUSTER__ macro
	result = strings.ReplaceAll(result, "__CLUSTER__", clusterName)

	// Substitute __STARTTIME__ macro
	if strings.Contains(result, "__STARTTIME__") {
		if m.startTime == "" {
			return "", fmt.Errorf("__STARTTIME__ macro found in query but start_time not configured")
		}
		startTimeValue, err := m.parseTimeExpression(m.startTime)
		if err != nil {
			return "", fmt.Errorf("failed to parse start_time: %w", err)
		}
		result = strings.ReplaceAll(result, "__STARTTIME__", fmt.Sprintf("%d", startTimeValue))
	}

	// Substitute __ENDTIME__ macro
	if strings.Contains(result, "__ENDTIME__") {
		if m.endTime == "" {
			return "", fmt.Errorf("__ENDTIME__ macro found in query but end_time not configured")
		}
		endTimeValue, err := m.parseTimeExpression(m.endTime)
		if err != nil {
			return "", fmt.Errorf("failed to parse end_time: %w", err)
		}
		result = strings.ReplaceAll(result, "__ENDTIME__", fmt.Sprintf("%d", endTimeValue))
	}

	return result, nil
}

// parseTimeExpression parses time expressions like "NOW", "NOW-5min", "NOW+10sec"
func (m *MacroSubstituter) parseTimeExpression(expr string) (int64, error) {
	expr = strings.TrimSpace(expr)

	// Handle simple "NOW" case
	if strings.ToUpper(expr) == "NOW" {
		return time.Now().UnixMilli(), nil
	}

	// Handle "NOW ± Xmin" or "NOW ± Xsec" patterns
	nowPattern := regexp.MustCompile(`^NOW\s*([+-])\s*(\d+)\s*(MIN|SEC)$`)
	matches := nowPattern.FindStringSubmatch(strings.ToUpper(expr))

	if len(matches) != 4 {
		// Try to parse as direct unix timestamp
		if timestamp, err := strconv.ParseInt(expr, 10, 64); err == nil {
			return timestamp, nil
		}
		return 0, fmt.Errorf("invalid time expression: %s", expr)
	}

	operator := matches[1]
	valueStr := matches[2]
	unit := matches[3]

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value in time expression: %s", valueStr)
	}

	now := time.Now()
	var duration time.Duration

	switch strings.ToUpper(unit) {
	case "MIN":
		duration = time.Duration(value) * time.Minute
	case "SEC":
		duration = time.Duration(value) * time.Second
	default:
		return 0, fmt.Errorf("unsupported time unit: %s", unit)
	}

	var result time.Time
	if operator == "+" {
		result = now.Add(duration)
	} else {
		result = now.Add(-duration)
	}

	return result.UnixMilli(), nil
}

// ValidateTimeExpression validates a time expression without evaluating it
func ValidateTimeExpression(expr string) error {
	if expr == "" {
		return nil // Empty is valid (optional)
	}

	expr = strings.TrimSpace(expr)

	// Handle simple "NOW" case
	if strings.ToUpper(expr) == "NOW" {
		return nil
	}

	// Handle "NOW ± Xmin" or "NOW ± Xsec" patterns
	nowPattern := regexp.MustCompile(`^NOW\s*([+-])\s*(\d+)\s*(MIN|SEC)$`)
	if nowPattern.MatchString(strings.ToUpper(expr)) {
		return nil
	}

	// Try to parse as direct unix timestamp
	if _, err := strconv.ParseInt(expr, 10, 64); err == nil {
		return nil
	}

	return fmt.Errorf("invalid time expression: %s (expected formats: NOW, NOW±Xmin, NOW±Xsec, or unix timestamp)", expr)
}
