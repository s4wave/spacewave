package testbed_test

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/testbed"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/sirupsen/logrus"
)

// TestReleaseClosesVolume checks that Release returns only after the volume
// controller has closed its store, so a test may remove the files afterward.
func TestReleaseClosesVolume(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "volume.db")
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), testbed.WithVolumeConfig(&volume_bolt.Config{Path: path}))
	if err != nil {
		t.Fatal(err)
	}
	ref, _, err := tb.Volume.PutBlock(ctx, []byte("testbed release"), nil)
	if err != nil {
		t.Fatal(err)
	}

	tb.Release()
	if _, _, err := tb.Volume.GetBlock(ctx, ref); err == nil {
		t.Fatal("volume store still open after Release")
	}
}
