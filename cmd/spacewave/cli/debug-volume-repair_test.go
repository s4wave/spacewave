//go:build !js

package spacewave_cli

import (
	"context"
	"path/filepath"
	"testing"

	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestDebugVolumeRepair checks that the repair sweeps the blocks a bucket with
// named roots owns only directly, keeps the blocks its roots reach and the
// blocks of a bucket without named roots, and deletes the local proof keys. It
// also checks that the repair refuses an open volume.
func TestDebugVolumeRepair(t *testing.T) {
	// Open a fresh volume.
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	le := logrus.NewEntry(logrus.New())
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
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
	if err := vol.SetBucketRoots(ctx, "space", nil, []block.NamedRoot{{Name: "world", Ref: root}}); err != nil {
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
	if err := putKey(ctx, vol, proofKey, []byte("proof")); err != nil {
		t.Fatal(err)
	}

	// The repair refuses the volume while it is open.
	if err := runDebugVolumeRepair(ctx, path, true, "json"); err == nil {
		t.Fatal("repair ran on an open volume")
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Repair and compact the stopped volume.
	if err := runDebugVolumeRepair(ctx, path, true, "json"); err != nil {
		t.Fatal(err)
	}

	// Only the leaked root is gone.
	vol, err = volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path, NoGenerateKey: true, NoWriteKey: true})
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
	tx, err := volume_s4db.GetDB(vol).NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if found, err := tx.Exists(ctx, proofKey); err != nil || found {
		t.Errorf("local proof key exists = %v, %v after the repair", found, err)
	}
}

// putKey writes key directly into the database of vol.
func putKey(ctx context.Context, vol volume.Volume, key, value []byte) error {
	// Open a write transaction, discarded unless it commits.
	tx, err := volume_s4db.GetDB(vol).NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Set the key and commit.
	if err := tx.Set(ctx, key, value); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
