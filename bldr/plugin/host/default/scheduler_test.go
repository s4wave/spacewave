package plugin_host_default

import (
	"testing"

	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
)

func TestNewSchedulerConfigUsesSpacewavePlatformPolicy(t *testing.T) {
	conf := NewSchedulerConfig("engine", "plugin-host", "volume", "peer", true, false, false)

	got := conf.FilterPluginPlatformIDs("spacewave-app", []string{"js", plugin_host_scheduler.WebJSWASMPlatformID})
	want := []string{"js"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("spacewave-app platform ids: got %v, want %v", got, want)
	}

	got = conf.FilterPluginPlatformIDs("spacewave-v86", []string{"js", plugin_host_scheduler.WebJSWASMPlatformID})
	want = []string{"js", plugin_host_scheduler.WebJSWASMPlatformID}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("spacewave-v86 platform ids: got %v, want %v", got, want)
	}
}
