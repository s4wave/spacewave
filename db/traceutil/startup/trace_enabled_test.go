//go:build bldr_startup_trace && !tinygo

package startuptrace

import "testing"

// TestStartupTraceBuildEnabled verifies the opt-in owner is selected with the build tag.
func TestStartupTraceBuildEnabled(t *testing.T) {
	if !buildTagged {
		t.Fatal("startup trace owner unexpectedly disabled")
	}
}
