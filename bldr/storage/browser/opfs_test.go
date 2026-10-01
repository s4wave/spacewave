//go:build js

package browser_storage

import (
	"testing"

	volume_opfs "github.com/s4wave/spacewave/db/volume/js/opfs"
)

// TestOpfsStorageBuildsRuntimeConfig checks the product format and lock namespace.
func TestOpfsStorageBuildsRuntimeConfig(t *testing.T) {
	// Build the config for one store.
	conf, err := NewOpfsStorage("prefix/").BuildVolumeConfig("state", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Check its location, lock namespace, format, and driver.
	opfsConf, ok := conf.(*volume_opfs.Config)
	if !ok {
		t.Fatalf("BuildVolumeConfig returned %T, want *volume_opfs.Config", conf)
	}
	if got, want := opfsConf.GetRootPath(), "prefix/state"; got != want {
		t.Fatalf("RootPath = %q, want %q", got, want)
	}
	if got, want := opfsConf.GetLockPrefix(), opfsConf.GetRootPath(); got != want {
		t.Fatalf("LockPrefix = %q, want %q", got, want)
	}
	if got, want := opfsConf.GetStorageFormatVersion(), volume_opfs.StorageFormatVersion; got != want {
		t.Fatalf("StorageFormatVersion = %d, want %d", got, want)
	}
	if got, want := opfsConf.GetDriverMode(), "auto"; got != want {
		t.Fatalf("DriverMode = %q, want %q", got, want)
	}
}
