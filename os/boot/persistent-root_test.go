//go:build linux

package boot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// TestPersistentRootReopen walks real publication, read-only reopens, cancellation
// and poisoned disposable stores, including a missing payload descendant.
func TestPersistentRootReopen(t *testing.T) {
	// Seed an absent path, then reject reseeding without changing its identity.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	dir := filepath.Join(t.TempDir(), "image")
	path := filepath.Join(dir, "image.s4wave")
	if err := SeedRoot(ctx, le, dir, bytes.NewBufferString("retained executable")); err != nil {
		t.Fatal(err)
	}
	if err := SeedRoot(ctx, le, dir, bytes.NewBufferString("replacement")); !os.IsExist(err) {
		t.Fatalf("reseed returned %v", err)
	}

	// Keep independent identity and physical-byte evidence for subsequent reopens.
	before := hashImageFile(t, path)
	var id string
	var head *bucket.ObjectRef
	var descendant *block.BlockRef

	// Both reopens preserve exact bytes. Cancellation leaves the ordinary read
	// interfaces alive until explicit Close, even after file cursors were opened.
	for range 2 {
		access := block.NewAccessLogger()
		openCtx, cancel := context.WithCancel(block.WithAccessLogger(ctx, access))
		r, err := OpenRoot(openCtx, le, dir)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		if id == "" {
			id, head = r.volumeID, r.head.Clone()
		}
		if r.volumeID != id || !r.head.EqualsRef(head) {
			t.Fatal("reopen changed Volume identity or committed root")
		}
		file, err := r.Handle().Lookup(ctx, "payload")
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		if err := r.VerifyImage(ctx, bytes.NewBufferString("retained executable")); err != nil {
			t.Fatal(err)
		}
		data := make([]byte, 4096)
		if n, err := file.ReadAt(ctx, 0, data); err != nil || n != int64(len(data)) {
			t.Fatalf("read after caller cancellation = %d, %v", n, err)
		}
		file.Release()

		// Retain only the final actual payload block read for the corruption
		// adversary, never reconstruct the on-disk UnixFS chunk layout.
		reads := access.Snapshot().GetAccesses()
		if len(reads) == 0 || reads[len(reads)-1].GetRef().EqualsRef(head.GetRootRef()) {
			t.Fatal("payload read did not reach a descendant block")
		}
		descendant = reads[len(reads)-1].GetRef().Clone()
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		if hashImageFile(t, path) != before {
			t.Fatal("read-only reopen or cancellation changed the store")
		}
	}
	t.Log("CANCELLATION_LIFETIME_OK readers survived cancellation until explicit close; store preserved")

	// Use the actual writable Volume publication API only for adversarial setup.
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path, SingleProcess: true, NoGenerateKey: true, NoWriteKey: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if vol != nil {
			if err := vol.Close(); err != nil {
				t.Error(err)
			}
		}
	}()

	// Construct an absent root and its generated head without storing its blocks.
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
	err = vol.PublishAtomic(ctx, &block.AtomicPublication{
		BucketID: imageBucket, TrackGC: true, RootName: selectedImageRoot, Root: missing,
		Heads: []*block.AtomicHeadUpdate{{
			ObjectStoreID: imageHeadStore, Key: []byte(imageHeadKey),
			Replace: func(context.Context, []byte, bool) ([]byte, error) { return data, nil },
		}},
	})
	if !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("incomplete publication returned %v", err)
	}

	// Read the prior head, then deliberately bypass publication on disposable data.
	metadata, release, err := vol.AccessObjectStore(ctx, imageHeadStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	got, err := readImageHead(ctx, metadata)
	if err != nil || !got.EqualsRef(head) {
		t.Fatalf("incomplete publication changed the head: %v", err)
	}

	// Install a deliberately invalid head outside the atomic publication API.
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
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}
	vol = nil

	// Missing top-level roots are refused before constructing a World reader.
	before = hashImageFile(t, path)
	opened, err := OpenRoot(ctx, le, dir)
	if opened != nil {
		_ = opened.Close()
		t.Fatal("opened an incomplete selected root")
	}
	if err == nil || hashImageFile(t, path) != before {
		t.Fatalf("root refusal did not preserve the store: %v", err)
	}
	t.Log("INCOMPLETE_ROOT_REJECTED original publication retained; poisoned store preserved")

	// Restore only the disposable head, then physically remove the recorded
	// payload descendant while leaving the selected root and retention intact.
	vol, err = volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path, SingleProcess: true, NoGenerateKey: true, NoWriteKey: true})
	if err != nil {
		t.Fatal(err)
	}

	// Restore the generated selected-head metadata before deleting payload data.
	metadata, releaseHead, err := vol.AccessObjectStore(ctx, imageHeadStore, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseHead()
	data, err = (&world_block_engine.HeadState{HeadRef: head}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Restore only the selected head; durable root edges remain unchanged.
	tx, err = metadata.NewTransaction(ctx, true)
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

	// Bypass graph retention only on disposable data to simulate a lost descendant.
	if err := vol.Store.RmBlock(ctx, descendant); err != nil {
		t.Fatal(err)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}
	vol = nil

	// The root/retention gate must pass, but the real streaming verifier must
	// reject missing content. Neither open, verification nor close may write.
	before = hashImageFile(t, path)
	r, err := OpenRoot(ctx, le, dir)
	if err != nil {
		t.Fatalf("descendant adversary also damaged the top-level root: %v", err)
	}
	if !r.head.EqualsRef(head) || r.volumeID != id {
		t.Fatal("descendant adversary changed selection or identity")
	}

	// Refuse the missing bytes through the same verifier used before every mount.
	verifyErr := r.VerifyImage(ctx, bytes.NewBufferString("retained executable"))
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(verifyErr, block.ErrNotFound) {
		t.Fatalf("missing payload descendant returned %v", verifyErr)
	}
	if hashImageFile(t, path) != before {
		t.Fatal("missing-descendant refusal changed the incomplete store")
	}
	t.Log("INCOMPLETE_DESCENDANT_REJECTED selected root and retention intact; poisoned store preserved")
}

// hashImageFile hashes the physical store without collecting its bytes.
func hashImageFile(t *testing.T, path string) [sha256.Size]byte {
	// Stream a fresh file handle so open/close side effects enter the comparison.
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		t.Fatal(err)
	}
	return [sha256.Size]byte(hash.Sum(nil))
}
