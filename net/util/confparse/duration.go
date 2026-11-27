package confparse

import "time"

// ParseDuration parses a duration or returns 0, nil if empty.
func ParseDuration(dur string) (time.Duration, error) {
	if dur == "" {
		return 0, nil
	}
	return time.ParseDuration(dur)
}
