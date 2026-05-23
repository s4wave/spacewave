package space_test

import (
	"testing"

	"github.com/s4wave/spacewave/core/space"
)

func TestValidateSpaceName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"empty", "", true},
		{"too long", "a" + string(make([]byte, 64)), true},
		{"valid simple", "test", false},
		{"valid complex", "My Space 123", false},
		{"valid with dash", "test-space", false},
		{"valid with underscore", "test_space", false},
		{"starts with number", "1space", true},
		{"consecutive dashes", "test--space", true},
		{"consecutive spaces", "test  space", true},
		{"consecutive underscores", "test__space", true},
		{"ends with dash", "space-", true},
		{"ends with space", "space ", true},
		{"ends with underscore", "space_", true},
		{"special chars", "test@space", true},
		{"valid mixed", "My-Cool_Space 123", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := space.ValidateSpaceName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateSpaceName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestFixupSpaceName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"no spaces", "test", "test"},
		{"leading spaces", "  test", "test"},
		{"trailing spaces", "test  ", "test"},
		{"multiple spaces", "test   space", "test space"},
		{"mixed spaces", "  test  space  ", "test space"},
		{"already clean", "test space", "test space"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := space.FixupSpaceName(tt.input)
			if result != tt.expected {
				t.Fatalf("FixupSpaceName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
