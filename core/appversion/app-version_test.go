package appversion

import "testing"

func TestGetVersion(t *testing.T) {
	if got := GetVersion(); got != "0.1.0" {
		t.Fatalf("expected runtime version 0.1.0, got %q", got)
	}
}
