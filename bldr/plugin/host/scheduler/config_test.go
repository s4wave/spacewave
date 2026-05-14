package plugin_host_scheduler

import "testing"

func TestSpacewaveDefaultPlatformSelectionPoliciesFilterBrowserNativePlatform(t *testing.T) {
	conf := &Config{PlatformSelectionPolicies: SpacewaveDefaultPlatformSelectionPolicies()}

	for _, pluginID := range []string{"spacewave-app", "spacewave-web", "spacewave-notes"} {
		got := conf.FilterPluginPlatformIDs(pluginID, []string{"js", WebJSWASMPlatformID})
		want := []string{"js"}
		if len(got) != len(want) || got[0] != want[0] {
			t.Fatalf("%s platform ids: got %v, want %v", pluginID, got, want)
		}
	}

	for _, pluginID := range SpacewaveBrowserNativePluginIDs() {
		got := conf.FilterPluginPlatformIDs(pluginID, []string{"js", WebJSWASMPlatformID})
		want := []string{"js", WebJSWASMPlatformID}
		if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
			t.Fatalf("%s platform ids: got %v, want %v", pluginID, got, want)
		}
	}
}
