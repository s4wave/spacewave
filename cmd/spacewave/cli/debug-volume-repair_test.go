//go:build !js

package spacewave_cli

import (
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/bbolt"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/sirupsen/logrus"
)

// TestDebugVolumeRepair checks that the repair sweeps the blocks a bucket with
// named roots owns only directly, keeps the blocks its roots reach and the
// blocks of a bucket without named roots, and deletes the local proof keys.
func TestDebugVolumeRepair(t *testing.T) {
	// Open a fresh volume.
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	le := logrus.NewEntry(logrus.New())
	vol, err := volume_bolt.NewBolt(ctx, le, &volume_bolt.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}

	// Publish a root with a child, and leak a superseded root beside it.
	child, _, err := vol.PrepareOwnedBlock(ctx, "space", []byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	root, _, err := vol.PrepareOwnedBlock(ctx, "space", []byte("root"), &block.PutOpts{Refs: []*block.BlockRef{child}})
	if err != nil {
		t.Fatal(err)
	}
	if err := vol.SetBucketRoot(ctx, "space", "world", root); err != nil {
		t.Fatal(err)
	}
	leaked, _, err := vol.PrepareOwnedBlock(ctx, "space", []byte("superseded root"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Own a block directly in a bucket without named roots.
	direct, _, err := vol.PrepareOwnedBlock(ctx, "plain", []byte("direct"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Write a local proof key into an object store.
	proofKey := []byte("h/objs/store/so/space/ls/retention/" + sobject_world_engine.LocalProofKeyPrefix + "space/" + root.MarshalString())
	err = volume_bolt.GetBoltDB(vol).Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte("hydra")).Put(proofKey, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Repair and compact the stopped volume.
	if err := runDebugVolumeRepair(ctx, path, true, "json"); err != nil {
		t.Fatal(err)
	}

	// Only the leaked root is gone.
	vol, err = volume_bolt.NewBolt(ctx, le, &volume_bolt.Config{Path: path, NoGenerateKey: true, NoWriteKey: true})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	for _, tc := range []struct {
		name string
		ref  *block.BlockRef
		want bool
	}{
		{"published root", root, true},
		{"published child", child, true},
		{"direct block", direct, true},
		{"leaked root", leaked, false},
	} {
		found, err := vol.GetBlockExists(ctx, tc.ref)
		if err != nil {
			t.Fatal(err)
		}
		if found != tc.want {
			t.Errorf("%s exists = %v, want %v", tc.name, found, tc.want)
		}
	}

	// The proof key is gone.
	err = volume_bolt.GetBoltDB(vol).View(func(tx *bbolt.Tx) error {
		if tx.Bucket([]byte("hydra")).Get(proofKey) != nil {
			t.Error("local proof key survived the repair")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
