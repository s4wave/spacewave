package confparse

import "time"

// ParseDuration parses a duration or returns 0, nil if empty.
func ParseDuration(dur string) (time.Duration, error) {
	if dur == "" {
		return 0, nil
	}
	return time.ParseDuration(dur)
}

// MarshalDuration marshals a duration to a string.
func MarshalDuration(dur time.Duration, ignoreEmpty bool) string {
	if dur == 0 && !ignoreEmpty {
		return ""
	}
	return dur.String()
}
