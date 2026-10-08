package volume_rpc_client

import "testing"

// TestProxyVolumeConfigDisablesGC checks a proxy volume leaves collection to
// the controller of the served volume.
func TestProxyVolumeConfigDisablesGC(t *testing.T) {
	conf := newProxyVolumeConfig([]string{"alias"})
	if !conf.GCDisabled() {
		t.Fatalf("proxy volume gc interval = %q, want disabled", conf.GetGcIntervalDur())
	}
	if aliases := conf.GetVolumeIdAlias(); len(aliases) != 1 || aliases[0] != "alias" {
		t.Fatalf("proxy volume aliases = %v", aliases)
	}
}
