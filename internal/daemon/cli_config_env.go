package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func envOrDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value != "" {
		return value
	}
	return fallback
}

func envFloat64(key string, fallback float64) (float64, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0, fmt.Errorf("%s must be a number", key)
	}
	return parsed, nil
}

func envIntValue(key string) (int, bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return 0, false, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, true, fmt.Errorf("%s must be an integer", key)
	}
	return parsed, true, nil
}

func envDurationValue(key string, fallback time.Duration) (time.Duration, bool, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, false, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, true, fmt.Errorf("%s must be a duration", key)
	}
	return parsed, true, nil
}

func envBoolValue(key string) (bool, bool) {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if value == "" {
		return false, false
	}
	switch value {
	case "1", "true", "yes", "on":
		return true, true
	default:
		return false, true
	}
}

func stringValue(value *string, fallback string) string {
	if value == nil {
		return fallback
	}
	return strings.TrimSpace(*value)
}

func intValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func float64Value(value *float64, fallback float64) float64 {
	if value == nil {
		return fallback
	}
	return *value
}

func boolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

func durationValue(fieldName string, value *string, fallback time.Duration) (time.Duration, error) {
	if value == nil {
		return fallback, nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return 0, fmt.Errorf("%s must be a duration", fieldName)
	}
	parsed, err := time.ParseDuration(trimmed)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", fieldName, err)
	}
	return parsed, nil
}
