package testbed_test

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestReleaseClosesVolume checks that Release returns only after the volume
// controller has closed its store, so a test may remove the files afterward.
func TestReleaseClosesVolume(t *testing.T) {
	// Start a testbed with a file-backed volume whose closure can be observed.
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), testbed.WithVolumeConfig(&volume_s4db.Config{Path: path}))
	if err != nil {
		t.Fatal(err)
	}

	// Save a block to verify the volume works before testbed release.
	ref, _, err := tb.Volume.PutBlock(ctx, []byte("testbed release"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Release the testbed and require the volume store to be closed.
	tb.Release()
	if _, _, err := tb.Volume.GetBlock(ctx, ref); err == nil {
		t.Fatal("volume store still open after Release")
	}
}
