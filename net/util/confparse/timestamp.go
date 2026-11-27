package confparse

import (
	"strconv"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

// ParseTimestamp parses a timestamp string.
//
// The string can be either a unix time milliseconds or RFC3339 timestamp.
// Returns nil, nil if empty.
func ParseTimestamp(timestampStr string) (*timestamppb.Timestamp, error) {
	// Treat an empty timestamp setting as an absent value.
	if timestampStr == "" {
		return nil, nil
	}

	// Parse the timestamp as a quoted date or numeric Unix time.
	ts := &timestamppb.Timestamp{}
	jdat := []byte(strconv.Quote(timestampStr))
	if err := ts.UnmarshalJSON(jdat); err != nil {
		ts.Reset()
		if err := ts.UnmarshalJSON([]byte(timestampStr)); err != nil {
			return nil, err
		}
	}

	return ts, nil
}
