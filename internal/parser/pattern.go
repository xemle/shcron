package parser

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Regex to quickly check if a string looks like a cron pattern (contains spaces/stars/slashes)
var cronPatternRegex = regexp.MustCompile(`^(\*|\d+|\*/\d+)( (\*|\d+|\*/\d+)){4}$`) // Matches 5 fields of cron
var intervalPatternRegex = regexp.MustCompile(`^\d+[smhd]$`)                        // Matches e.g., "5s", "1m"

// IsCronPattern checks if the given string appears to be a cron expression.
// This is a heuristic check, not a full validation.
func IsCronPattern(pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	// Check if it has 5 fields separated by space
	parts := strings.Fields(pattern)
	if len(parts) == 5 {
		// Check for common cron characters like *, /, or numbers.
		// This is a rough check to distinguish from simple strings.
		for _, part := range parts {
			if strings.ContainsAny(part, "*/,") || isNumeric(part) {
				continue
			} else {
				return false // Not a cron-like part
			}
		}
		return true
	}
	// Also check for predefined cron strings like "@hourly", "@daily"
	// The robfig/cron library supports these, but our simple CLI might not need to distinguish them
	// beyond sending them to the cron parser if they don't match our simple interval.
	if strings.HasPrefix(pattern, "@") {
		return true
	}

	// More robust regex check
	return cronPatternRegex.MatchString(pattern)
}

func isNumeric(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil
}

// ParseIntervalPattern parses a simple interval pattern string (e.g., "5s", "1m", "2h", "1d")
// into a time.Duration.
func ParseIntervalPattern(pattern string) (time.Duration, error) {
	pattern = strings.ToLower(strings.TrimSpace(pattern))

	if !intervalPatternRegex.MatchString(pattern) {
		return 0, fmt.Errorf("invalid interval pattern format: %s. Expected format like '5s', '1m', '2h', '1d'", pattern)
	}

	unitChar := pattern[len(pattern)-1]
	valueStr := pattern[:len(pattern)-1]

	value, err := strconv.Atoi(valueStr)
	if err != nil { // Should not happen due to regex check, but good for safety
		return 0, fmt.Errorf("invalid number in pattern: %s", valueStr)
	}
	if value <= 0 {
		return 0, fmt.Errorf("interval value must be positive: %d", value)
	}

	switch unitChar {
	case 's':
		return time.Duration(value) * time.Second, nil
	case 'm':
		return time.Duration(value) * time.Minute, nil
	case 'h':
		return time.Duration(value) * time.Hour, nil
	case 'd':
		return time.Duration(value) * 24 * time.Hour, nil // 1 day = 24 hours
	default: // Should not happen due to regex check
		return 0, fmt.Errorf("invalid time unit: %c. Supported units: s, m, h, d", unitChar)
	}
}
