package confparse

import (
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    *timestamppb.Timestamp
		expectError bool
	}{
		{
			name:        "EmptyString",
			input:       "",
			expected:    nil,
			expectError: false,
		},
		{
			name:  "ValidUnixMilliseconds",
			input: "1629048153000",
			expected: &timestamppb.Timestamp{
				Seconds: 1629048153,
			},
			expectError: false,
		},
		{
			name:  "ValidRFC3339",
			input: "2021-08-15T15:49:13Z",
			expected: &timestamppb.Timestamp{
				Seconds: 1629042553,
			},
			expectError: false,
		},
		{
			name:        "InvalidInput",
			input:       "invalid",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := ParseTimestamp(tt.input)
			if tt.expectError {
				if err == nil {
					t.FailNow()
				}
			} else {
				if err != nil {
					t.Fatal(err.Error())
				}
			}
			if !tt.expected.EqualVT(actual) {
				t.Fatalf("expected %q got %q", tt.expected, actual)
			}
		})
	}
}
