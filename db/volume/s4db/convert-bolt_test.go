//go:build !js && !wasip1

package volume_s4db_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestConvertBolt checks that opening a bolt Volume file converts it in
// place, keeping the Volume key and blocks, and that the next open uses the
// converted file. A conversion file beside the bolt file that no claim
// accounts for is kept and refuses the conversion.
func TestConvertBolt(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	t.Run("convert", func(t *testing.T) {
		// Create a bolt Volume, then open it twice as an s4db Volume: once
		// converting, once reading.
		path, id, refs := newBoltVolume(t)
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
	})

	t.Run("unowned converting", func(t *testing.T) {
		// Put a file no claim accounts for where the copy goes.
		path, _, _ := newBoltVolume(t)
		other := []byte("someone else's file")
		if err := os.WriteFile(path+".converting", other, 0o600); err != nil {
			t.Fatal(err)
		}

		// The conversion refuses permanently and keeps the file.
		_, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		if !volume.IsPermanent(err) {
			t.Fatalf("open beside an unowned file: %v, want a permanent error", err)
		}
		got, err := os.ReadFile(path + ".converting")
		if err != nil || !bytes.Equal(got, other) {
			t.Fatalf("unowned file changed: %v", err)
		}

		// Once it is moved aside, the bolt file converts.
		if err := os.Remove(path + ".converting"); err != nil {
			t.Fatal(err)
		}
		vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		if err := vol.Close(); err != nil {
			t.Fatal(err)
		}
	})
}

// newBoltVolume creates a bolt Volume holding ten blocks and closes it. It
// returns the file's path, the Volume's peer ID and the block refs.
func newBoltVolume(t *testing.T) (string, peer.ID, []*block.BlockRef) {
	t.Helper()
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	bolt, err := volume_bolt.NewBolt(ctx, logrus.NewEntry(logrus.New()), &volume_bolt.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	id := bolt.GetPeerID()
	refs := make([]*block.BlockRef, 10)
	for i := range refs {
		refs[i], _, err = bolt.PutBlock(ctx, writerBlock("bolt", i), &block.PutOpts{RefsKnown: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := bolt.Close(); err != nil {
		t.Fatal(err)
	}
	return path, id, refs
}
