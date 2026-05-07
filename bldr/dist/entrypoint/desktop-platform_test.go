package dist_entrypoint

import "testing"

func TestIsDesktopPlatformID(t *testing.T) {
	tests := []struct {
		platformID string
		want       bool
	}{
		{"desktop", true},
		{"desktop/darwin/arm64", true},
		{"desktop/linux/amd64", true},
		{"desktop/windows/arm64", true},
		{"js", false},
		{"web/js/wasm", false},
		{"browser", false},
		{"notdesktop/darwin/arm64", false},
	}

	for _, test := range tests {
		got := isDesktopPlatformID(test.platformID)
		if got != test.want {
			t.Fatalf("isDesktopPlatformID(%q) = %v, want %v", test.platformID, got, test.want)
		}
	}
}
