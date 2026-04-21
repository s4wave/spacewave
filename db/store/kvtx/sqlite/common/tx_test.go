package common

import (
	"bytes"
	"testing"
)

// TestCreateUpperBound verifies prefix upper bounds for SQLite range scans.
func TestCreateUpperBound(t *testing.T) {
	tests := []struct {
		name   string
		prefix []byte
		expect []byte
	}{
		{
			name:   "single byte",
			prefix: []byte("t"),
			expect: []byte("u"),
		},
		{
			name:   "carry trims suffix",
			prefix: []byte{0x12, 0xff},
			expect: []byte{0x13},
		},
		{
			name:   "all max returns nil",
			prefix: []byte{0xff, 0xff},
			expect: nil,
		},
	}

	for _, tc := range tests {
		if got := CreateUpperBound(tc.prefix); !bytes.Equal(got, tc.expect) {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.expect, got)
		}
	}
}
