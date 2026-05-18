package dist_entrypoint

import "testing"

// TestStorageRootEnvVar tests the env var is consistent.
//
// Think twice before changing this.
func TestStorageRootEnvVar(t *testing.T) {
	if StorageRootEnvVar("bldr-demo") != "BLDR_DEMO_DATA_DIR" {
		t.FailNow()
	}
	if StorageRootEnvVar("aperture-alpha") != "APERTURE_ALPHA_DATA_DIR" {
		t.FailNow()
	}
}

func TestDistStorageVolumeConfigDisablesGC(t *testing.T) {
	conf := newDistStorageVolumeConfig("storage", "spacewave")
	if conf.GetStorageId() != "storage" {
		t.Fatalf("storage id = %q, want storage", conf.GetStorageId())
	}
	if conf.GetStorageVolumeId() != "dist/spacewave" {
		t.Fatalf("storage volume id = %q, want dist/spacewave", conf.GetStorageVolumeId())
	}

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
	if !volConf.GetDisablePeer() {
		t.Fatal("expected peer disabled")
	}
	aliases := volConf.GetVolumeIdAlias()
	if len(aliases) != 1 || aliases[0] != "dist" {
		t.Fatalf("aliases = %v, want [dist]", aliases)
	}
}
