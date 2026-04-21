package unixfs_block

import (
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// ToTimestamp converts a time.Time into a Timestamp.
// If ts is zero, returns nil.
func ToTimestamp(ts time.Time, fillPlaceholder bool) *timestamppb.Timestamp {
	var now *timestamppb.Timestamp
	if !ts.IsZero() {
		now = timestamppb.New(ts)
	}
	if fillPlaceholder {
		now = FillPlaceholderTimestamp(now)
	}
	return now
}

// FillPlaceholderTimestamp fills a timestamp with a placeholder if nil.
func FillPlaceholderTimestamp(ts *timestamppb.Timestamp) *timestamppb.Timestamp {
	if ts == nil {
		// placeholderMtime is a placeholder used for mtime, ctime, atime
		placeholderMtime := time.Date(2021, time.January, 1, 0, 0, 0, 0, time.UTC)
		ts = timestamppb.New(placeholderMtime)
	}

	return ts
}
