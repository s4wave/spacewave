package plugin_host_default

import (
	"slices"
	"testing"

	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
)

func TestNewNativeDesktopSchedulerConfigRestrictsJSPlatform(t *testing.T) {
	// Configure the native desktop scheduler with selected JavaScript-capable plugins.
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

	// Verify the core plugin selects only the native desktop platform.
	hostPlatforms := []string{"desktop/darwin/arm64", bldr_platform.PlatformID_JS}
	got := conf.FilterPluginPlatformIDs("spacewave-core", hostPlatforms)
	want := []string{"desktop/darwin/arm64"}
	if !slices.Equal(got, want) {
		t.Fatalf("native-only plugin platforms = %v, want %v", got, want)
	}

	// Verify an allowed application plugin retains its JavaScript platform.
	got = conf.FilterPluginPlatformIDs("spacewave-app", hostPlatforms)
	want = []string{"desktop/darwin/arm64", bldr_platform.PlatformID_JS}
	if !slices.Equal(got, want) {
		t.Fatalf("quickjs plugin platforms = %v, want %v", got, want)
	}
}
