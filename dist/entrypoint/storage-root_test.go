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
