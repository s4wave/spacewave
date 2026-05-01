package appversion

import (
	"strings"
	"testing"
)

func TestGetVersion(t *testing.T) {
	want := strings.TrimSpace(versionText)
	if got := GetVersion(); got != want {
		t.Fatalf("expected runtime version %q, got %q", want, got)
	}
}
