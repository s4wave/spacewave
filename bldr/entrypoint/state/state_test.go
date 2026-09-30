package state

import (
	"testing"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// TestVolumeConfig keeps catalog and host aliases on one store without deleting plugin blocks.
func TestVolumeConfig(t *testing.T) {
	// Build a volume config for the storage id.
	conf := NewVolumeConfig("storage")
	if conf.GetStorageId() != "storage" {
		t.Fatalf("storage id = %q, want storage", conf.GetStorageId())
	}
	if conf.GetStorageVolumeId() != VolumeID {
		t.Fatalf("storage volume id = %q, want state", conf.GetStorageVolumeId())
	}

	// The volume config must disable GC and keep plugin and dist aliases.
	volConf := conf.GetVolumeConfig()
	if volConf.GetGcIntervalDur() != "0" {
		t.Fatalf("gc interval = %q, want disabled", volConf.GetGcIntervalDur())
	}
	if !volConf.GCDisabled() {
		t.Fatal("expected volume GC disabled")
	}
	if !volConf.GetDisableEventBlockRm() {
		t.Fatal("expected event block remove disabled")
	}
	if volConf.GetDisablePeer() {
		t.Fatal("expected native peer services enabled")
	}
	aliases := volConf.GetVolumeIdAlias()
	if len(aliases) != 2 || aliases[0] != bldr_plugin.PluginVolumeID || aliases[1] != "dist" {
		t.Fatalf("aliases = %v, want [plugin-host dist]", aliases)
	}
}
