package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseUntilDate attempts to parse the --until string into a time.Time.
// It supports various relative and absolute formats.
func ParseUntilDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	now := time.Now()

	// Try absolute formats first
	layouts := []string{
		"2006-01-02 15:04:05",
		"2006-01-02 15:04",
		"2006-01-02",
		time.RFC3339,
		time.RFC1123Z,
		time.Kitchen,
		time.Stamp,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}

	// Try relative formats
	parts := strings.Fields(s)
	if len(parts) == 2 && parts[0] == "in" {
		durationStr := parts[1]
		// Try parsing directly as a duration (e.g., "5s", "1h30m")
		if d, err := time.ParseDuration(durationStr); err == nil {
			return now.Add(d), nil
		}

		// Fallback to specific unit parsing for clarity/simplicity
		if strings.HasSuffix(durationStr, "s") {
			val, err := strconv.Atoi(strings.TrimSuffix(durationStr, "s"))
			if err == nil {
				return now.Add(time.Duration(val) * time.Second), nil
			}
		} else if strings.HasSuffix(durationStr, "m") {
			val, err := strconv.Atoi(strings.TrimSuffix(durationStr, "m"))
			if err == nil {
				return now.Add(time.Duration(val) * time.Minute), nil
			}
		} else if strings.HasSuffix(durationStr, "h") {
			val, err := strconv.Atoi(strings.TrimSuffix(durationStr, "h"))
			if err == nil {
				return now.Add(time.Duration(val) * time.Hour), nil
			}
		} else if strings.HasSuffix(durationStr, "d") {
			val, err := strconv.Atoi(strings.TrimSuffix(durationStr, "d"))
			if err == nil {
				return now.Add(time.Duration(val) * 24 * time.Hour), nil
			}
		}
	} else if s == "next day" {
		// Next midnight (00:00:00 of the next day)
		return now.Add(24 * time.Hour).Truncate(24 * time.Hour), nil
	} else if s == "next hour" {
		// Next top of the hour (HH:00:00 of the current hour + 1)
		return now.Add(time.Hour).Truncate(time.Hour), nil
	}

	return time.Time{}, fmt.Errorf("unsupported date/time format")
}
