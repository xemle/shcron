package schedule

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/araddon/dateparse"
)

var intervalRe = regexp.MustCompile(`^(\d+)([smhd])$`)

// IsCronPattern checks if the given string resembles a cron pattern.
// It primarily checks for the number of fields.
func IsCronPattern(pattern string) bool {
	fields := strings.Fields(pattern)
	return (len(fields) >= 5 && len(fields) <= 6) || strings.HasPrefix(pattern, "@every")
}

// ParseIntervalPattern parses a string like "5s", "10m", "2h", "3d" into a time.Duration.
func ParseIntervalPattern(pattern string) (time.Duration, error) {
	value, err := time.ParseDuration(pattern)
	if err != nil {
		return 0, fmt.Errorf("invalid numeric value in interval: %s", pattern)
	}
	return value, nil
}

// ParseUntilDate parses a flexible date string using dateparse.
// Note: github.com/araddon/dateparse typically uses time.Now() for relative terms
// and time.Local for timezone context. Direct injection of `baseTime` for relative parsing
// is not a standard feature of its public API.
func ParseUntilDate(dateStr string, baseTime time.Time) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, nil // Empty string means no until date
	}

	// For github.com/araddon/dateparse, the 'baseTime' parameter for relative
	// parsing is implicitly time.Now() (or time.Local if configured).
	// We cannot directly pass a 'baseTime' to dateparse.ParseStrict or ParseAny
	// to make them parse relative terms like "next day" or "in 1 hour" relative
	// to 'baseTime'. The 'PreferredDate' option within ParseOptions is also
	// for resolving ambiguities (like "Jan 1" could be this year or next),
	// not for setting the base for relative terms.

	// Therefore, the `baseTime` parameter for this function becomes largely
	// advisory for absolute dates, and `dateparse` will use `time.Now()` for
	// relative ones. For robust testing of "relative to baseTime" behavior,
	// you might need a different parsing library or to temporarily mock `time.Now()`
	// for the entire process during tests.

	// No options are passed here as dateparse doesn't have public API for
	// `WithCurrent` or `WithPreferredDate` functions.
	parsedTime, err := dateparse.ParseStrict(dateStr)
	if err != nil {
		// If strict parsing fails, try non-strict as a fallback for broader acceptance
		parsedTime, err = dateparse.ParseAny(dateStr)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to parse date string '%s': %w", dateStr, err)
		}
	}
	return parsedTime, nil
}
