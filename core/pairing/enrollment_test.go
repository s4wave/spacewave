package pairing

import (
	"strings"
	"testing"
)

// TestCleanLabel checks that a label from another client is reduced to
// bounded printable text before it is shown or stored.
func TestCleanLabel(t *testing.T) {
	long := strings.Repeat("a", maxLabelRunes+10)
	for _, tc := range []struct {
		in, want string
	}{
		{"Claude Code on build-box", "Claude Code on build-box"},
		{"  Codex\x1b[31m on\nhost ", "Codex[31m onhost"},
		{long, long[:maxLabelRunes]},
		{"\x00\x07", ""},
	} {
		if got := cleanLabel(tc.in); got != tc.want {
			t.Errorf("cleanLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
