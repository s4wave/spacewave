//go:build !js && !wasip1

package volume_s4db_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestConvertBolt checks that opening a bolt Volume file converts it in
// place, keeping the Volume key and blocks, and that the next open uses the
// converted file.
func TestConvertBolt(t *testing.T) {
	// Create a bolt Volume.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	bolt, err := volume_bolt.NewBolt(ctx, le, &volume_bolt.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}

	// Write blocks to it and close it.
	id := bolt.GetPeerID()
	refs := make([]*block.BlockRef, 10)
	for i := range refs {
		refs[i], _, err = bolt.PutBlock(ctx, writerBlock("bolt", i), nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := bolt.Close(); err != nil {
		t.Fatal(err)
	}

	// Open it twice as an s4db Volume: once converting, once reading.
	for range 2 {
		vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		if got := vol.GetPeerID(); got != id {
			t.Fatalf("converted volume peer %s, want %s", got, id)
		}
		for i, ref := range refs {
			got, found, err := vol.GetBlock(ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			if !found || !bytes.Equal(got, writerBlock("bolt", i)) {
				t.Fatalf("converted block %d found %v", i, found)
			}
		}
		if err := vol.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// Only the converted file remains.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != filepath.Base(path) {
			t.Errorf("leftover file %q", e.Name())
		}
	}
}
