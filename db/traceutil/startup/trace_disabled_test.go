//go:build !bldr_startup_trace || tinygo

package startuptrace

import "testing"

// TestStartupTraceBuildDisabled verifies the production owner is no-op without the opt-in tag.
func TestStartupTraceBuildDisabled(t *testing.T) {
	if buildTagged {
		t.Fatal("startup trace owner unexpectedly enabled")
	}
}
