//go:build linux

package boot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// TestPersistentRootReopen walks real seeding, retained publication, two reopens,
// a refused incomplete publication and a deliberately poisoned disposable head.
func TestPersistentRootReopen(t *testing.T) {
	// Seed an absent path, then reject reseeding without changing its identity.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	dir := filepath.Join(t.TempDir(), "image")
	if err := SeedRoot(ctx, le, dir, bytes.NewBufferString("retained executable")); err != nil {
		t.Fatal(err)
	}
	if err := SeedRoot(ctx, le, dir, bytes.NewBufferString("replacement")); !os.IsExist(err) {
		t.Fatalf("reseed returned %v", err)
	}
	var id string
	var head *bucket.ObjectRef

	// Each open reads the committed content and the same durable retained root.
	for range 2 {
		r, err := OpenRoot(ctx, le, dir)
		if err != nil {
			t.Fatal(err)
		}
		if id == "" {
			id, head = r.volume.GetID(), r.engine.GetRootRef()
		}
		if r.volume.GetID() != id || !r.engine.GetRootRef().EqualsRef(head) {
			t.Fatal("reopen changed Volume identity or committed root")
		}
		if err := r.VerifyImage(ctx, bytes.NewBufferString("retained executable")); err != nil {
			t.Error(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
	}

	// Try to publish a head whose root block has never been written.
	r, err := OpenRoot(ctx, le, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if r != nil {
			if err := r.Close(); err != nil {
				t.Error(err)
			}
		}
	}()

	// Build a valid reference without writing the corresponding block.
	missing, err := block.BuildBlockRef([]byte("absent World root"), nil)
	if err != nil {
		t.Fatal(err)
	}
	incomplete := head.Clone()
	incomplete.RootRef = missing
	data, err := (&world_block_engine.HeadState{HeadRef: incomplete}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Use the actual Volume publication interface, not an injected test seam.
	publisher := r.volume.(block.AtomicPublisher)
	err = publisher.PublishAtomic(ctx, &block.AtomicPublication{
		BucketID: imageBucket, TrackGC: true, RootName: selectedImageRoot, Root: missing,
		Heads: []*block.AtomicHeadUpdate{{
			ObjectStoreID: imageHeadStore, Key: []byte(imageHeadKey),
			Replace: func(context.Context, []byte, bool) ([]byte, error) {
				return data, nil
			},
		}},
	})
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("incomplete publication returned %v", err)
	}

	// Read metadata on the already-open Volume; s4db rejects a second handle
	// to the same file in this process, including a read-only one.
	metadata, release, err := r.volume.AccessObjectStore(ctx, imageHeadStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Failed publication must leave the original discoverable head unchanged.
	got, err := readImageHead(ctx, metadata)
	if err != nil || !got.EqualsRef(head) {
		t.Fatalf("incomplete publication changed the head: %v", err)
	}
	if err := r.VerifyImage(ctx, bytes.NewBufferString("retained executable")); err != nil {
		t.Fatal(err)
	}

	// Deliberately bypass publication on this disposable store to falsify boot's
	// readiness gate. It must refuse the incomplete root, never rebuild it.
	tx, err := metadata.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Set(ctx, []byte(imageHeadKey), data); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Close the disposable database before checking the refusal is read-only.
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	r = nil

	// Byte equality proves the refused open did not recover, migrate or reseed.
	path := filepath.Join(dir, "image.s4wave")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	opened, err := OpenRoot(ctx, le, dir)
	if opened != nil {
		_ = opened.Close()
		t.Fatal("opened an incomplete selected root")
	}
	if err == nil {
		t.Fatal("accepted an incomplete selected root")
	}

	// A failed open must leave even the poisoned bytes intact.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("refused open changed the incomplete store")
	}
	t.Log("INCOMPLETE_ROOT_REJECTED original publication retained; poisoned store preserved")
}
