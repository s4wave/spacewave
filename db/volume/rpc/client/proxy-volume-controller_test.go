package volume_rpc_client

import "testing"

// TestProxyVolumeConfigTracksGC checks the bucket handles of a proxy volume
// track references and the proxy volume leaves collection to the served volume.
func TestProxyVolumeConfigTracksGC(t *testing.T) {
	conf := newProxyVolumeConfig([]string{"alias"})
	if conf.GCDisabled() {
		t.Fatal("proxy volume bucket handles do not track references")
	}
	if aliases := conf.GetVolumeIdAlias(); len(aliases) != 1 || aliases[0] != "alias" {
		t.Fatalf("proxy volume aliases = %v", aliases)
	}
	if _, ok := any(&ProxyVolume{}).(interface{ DelegatesGC() }); !ok {
		t.Fatal("proxy volume collects the served volume")
	}
}
