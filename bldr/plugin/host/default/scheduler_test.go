package plugin_host_default

import (
	"slices"
	"testing"

	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
)

func TestNewNativeDesktopSchedulerConfigRestrictsJSPlatform(t *testing.T) {
	conf := NewNativeDesktopSchedulerConfig(
		"",
		"engine",
		"plugin-host",
		"volume",
		"peer",
		true,
		true,
		true,
		[]string{"spacewave-app", "spacewave-web"},
	)

	hostPlatforms := []string{"desktop/darwin/arm64", bldr_platform.PlatformID_JS}
	got := conf.FilterPluginPlatformIDs("spacewave-core", hostPlatforms)
	want := []string{"desktop/darwin/arm64"}
	if !slices.Equal(got, want) {
		t.Fatalf("native-only plugin platforms = %v, want %v", got, want)
	}

	got = conf.FilterPluginPlatformIDs("spacewave-app", hostPlatforms)
	want = []string{"desktop/darwin/arm64", bldr_platform.PlatformID_JS}
	if !slices.Equal(got, want) {
		t.Fatalf("quickjs plugin platforms = %v, want %v", got, want)
	}
}
