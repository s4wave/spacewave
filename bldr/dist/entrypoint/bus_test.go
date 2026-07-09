package dist_entrypoint

import "testing"

func TestIsWebDistPlatform(t *testing.T) {
	for _, tt := range []struct {
		platformID string
		want       bool
	}{
		{platformID: "js", want: true},
		{platformID: "web/js/wasm", want: true},
		{platformID: "desktop/js/wasm", want: true},
		{platformID: "desktop/darwin/arm64", want: false},
		{platformID: "linux/amd64", want: false},
	} {
		if got := isWebDistPlatform(tt.platformID); got != tt.want {
			t.Fatalf("isWebDistPlatform(%q) = %v, want %v", tt.platformID, got, tt.want)
		}
	}
}
