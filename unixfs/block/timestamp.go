package unixfs_block

import (
	"time"

	timestamp "github.com/aperturerobotics/timestamp"
)

// ToTimestamp converts a time.Time into a Timestamp.
// If ts is zero, returns nil.
func ToTimestamp(ts time.Time, fillPlaceholder bool) *timestamp.Timestamp {
	var now *timestamp.Timestamp
	if !ts.IsZero() {
		nt := timestamp.ToTimestamp(ts)
		now = &nt
	}
	if fillPlaceholder {
		now = FillPlaceholderTimestamp(now)
	}
	return now
}

// FillPlaceholderTimestamp fills a timestamp with a placeholder if nil.
func FillPlaceholderTimestamp(ts *timestamp.Timestamp) *timestamp.Timestamp {
	if ts == nil {
		todoTs := timestamp.ToTimestamp(TodoMtime)
		ts = &todoTs
	}
	return ts
}
