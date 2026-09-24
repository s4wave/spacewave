package dist_entrypoint

import (
	"slices"
	"testing"
)

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

func TestReleaseSchedulerConfigCachesReleaseWorld(t *testing.T) {
	conf := newReleaseSchedulerConfig(
		"project",
		"engine",
		"plugin-host",
		"volume",
		"peer",
	)

	if slices.Contains(conf.GetNoCopyBucketIds(), "spacewave-release") {
		t.Fatalf("release scheduler no-copy bucket IDs = %v, must not suppress spacewave-release", conf.GetNoCopyBucketIds())
	}
}
