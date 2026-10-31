package block_store_writeback

import (
	"context"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	block_store_testing "github.com/s4wave/spacewave/db/block/store/test"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// failingStore rejects every batch write, standing in for an unreachable bucket.
type failingStore struct {
	block.StoreOps
}

// PutBlockBatch always fails.
func (f *failingStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, errors.New("bucket unreachable")
}

// leafStore tracks every block as a leaf, as a GC store does for a block with
// no outgoing refs. The bare in-memory store holds bytes without refs.
type leafStore struct {
	block.StoreOps
}

// GetStoredBlock reads the block as a known leaf.
func (l *leafStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := block.GetBlockWithoutRefs(ctx, l.StoreOps, ref)
	if stored != nil {
		stored.RefsKnown = true
	}
	return stored, err
}

func newInmemBlockStore() block.StoreOps {
	return &leafStore{StoreOps: newUntrackedBlockStore()}
}

// newUntrackedBlockStore returns a store that holds bytes without refs.
func newUntrackedBlockStore() block.StoreOps {
	return block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
}

// TestUploadAfterRestart checks that queued writes survive a failed upload and
// a reopen, then drain to the remote store.
func TestUploadAfterRestart(t *testing.T) {
	// Prepare local block storage and durable upload markers.
	ctx := t.Context()
	local := newInmemBlockStore()
	markers := store_kvtx_inmem.NewStore()

	// Unplaced writes are not queued.
	s, err := NewStore(ctx, local, local, markers, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PutBlock(ctx, []byte("unplaced"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}

	// Two distinct writes and one repeat queue two blocks.
	var refs []*block.BlockRef
	for _, data := range []string{"alpha", "beta", "alpha"} {
		ref, _, err := s.PutBlock(ctx, []byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	if status, _ := s.GetStatus(); status.Pending != 2 || status.PendingBytes != 9 {
		t.Fatalf("pending %d (%d bytes), expected 2 (9 bytes)", status.Pending, status.PendingBytes)
	}

	// A failing remote keeps the markers and reports the failure.
	if err := s.Upload(ctx, &failingStore{StoreOps: newInmemBlockStore()}); err == nil {
		t.Fatal("expected upload failure")
	}
	if status, _ := s.GetStatus(); status.Pending != 2 || status.Err == nil {
		t.Fatalf("after failure: pending %d, err %v", status.Pending, status.Err)
	}

	// A reopened store recounts the markers and drains them.
	s, err = NewStore(ctx, local, local, markers, true)
	if err != nil {
		t.Fatal(err)
	}

	// Drain restored upload markers into the remote block store.
	remote := newInmemBlockStore()
	uploadCtx, uploadCancel := context.WithCancel(ctx)
	defer uploadCancel()
	done := make(chan error, 1)
	go func() { done <- s.Upload(uploadCtx, remote) }()
	for {
		status, changed := s.GetStatus()
		if status.Pending == 0 {
			if status.Err != nil {
				t.Fatalf("status error after drain: %v", status.Err)
			}
			if status.Uploaded != 2 || status.UploadedBytes != 9 || status.UploadReads < 1 {
				t.Fatalf("drain counted %d blocks, %d bytes, %d reads", status.Uploaded, status.UploadedBytes, status.UploadReads)
			}
			break
		}
		<-changed
	}

	// Stop the upload routine after the restored queue drains.
	uploadCancel()
	<-done

	// Require the remote store to contain both queued blocks.
	for _, ref := range refs[:2] {
		found, err := remote.GetBlockExists(ctx, ref)
		if err != nil || !found {
			t.Fatalf("remote missing %s: %v", ref.MarshalString(), err)
		}
	}
}

// TestDisableDropsMarkers checks that disabling upload clears the queue.
func TestDisableDropsMarkers(t *testing.T) {
	// Create an enabled upload store over durable markers.
	ctx := t.Context()
	markers := store_kvtx_inmem.NewStore()
	s, err := NewStore(ctx, newInmemBlockStore(), nil, markers, true)
	if err != nil {
		t.Fatal(err)
	}

	// Queue a block and disable uploads to drop its marker.
	if _, _, err := s.PutBlock(ctx, []byte("queued"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Require a reopened upload store to retain the empty queue.
	reopened, err := NewStore(ctx, newInmemBlockStore(), nil, markers, true)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := reopened.GetStatus(); status.Pending != 0 {
		t.Fatalf("pending %d after disable, expected 0", status.Pending)
	}
}

// TestBackfillQueuesStoredBlocks checks that choosing a remote queues the
// blocks stored before it, once per remote, across a reopen.
func TestBackfillQueuesStoredBlocks(t *testing.T) {
	// Create local block storage with uploads initially disabled.
	ctx := t.Context()
	local := newInmemBlockStore()
	markers := store_kvtx_inmem.NewStore()
	s, err := NewStore(ctx, local, local, markers, false)
	if err != nil {
		t.Fatal(err)
	}

	// Two blocks are stored while uploads are off, and a third ref is listed
	// but not stored.
	var refs []*block.BlockRef
	for _, data := range []string{"before-a", "before-b"} {
		ref, _, err := s.PutBlock(ctx, []byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	missing, _, err := newInmemBlockStore().PutBlock(ctx, []byte("elsewhere"), nil)
	if err != nil {
		t.Fatal(err)
	}
	var listed int
	list := func(context.Context) ([]*block.BlockRef, error) {
		listed++
		return append(refs, missing), nil
	}

	// Backfill the selected remote target with the existing local blocks.
	if err := s.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := s.Backfill(ctx, "bucket", list); err != nil {
		t.Fatal(err)
	}
	status, _ := s.GetStatus()
	if status.Pending != 2 || status.PendingBytes != 16 || status.Target != "bucket" {
		t.Fatalf("after backfill: %+v", status)
	}

	// A reopened store remembers the completed backfill.
	s, err = NewStore(ctx, local, local, markers, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Backfill(ctx, "bucket", list); err != nil {
		t.Fatal(err)
	}
	if listed != 1 {
		t.Fatalf("listed %d times, expected 1", listed)
	}

	// Disabling forgets it, so the next remote backfills again.
	if err := s.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	if status, _ := s.GetStatus(); status.Target != "" {
		t.Fatalf("target %q after disable", status.Target)
	}
}

// TestUploadUntrackedBlock checks that a block held without its refs fails the
// upload and stays queued, since the remote would keep it as a leaf.
func TestUploadUntrackedBlock(t *testing.T) {
	// Queue a block that the local store holds without its refs.
	ctx := t.Context()
	s, err := NewStore(ctx, newUntrackedBlockStore(), nil, store_kvtx_inmem.NewStore(), true)
	if err != nil {
		t.Fatal(err)
	}
	ref, _, err := s.PutBlock(ctx, []byte("untracked"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// The upload fails naming the block, and nothing reaches the remote.
	remote := newInmemBlockStore()
	err = s.Upload(ctx, remote)
	if !errors.Is(err, block.ErrRefsUnknown) || !strings.Contains(err.Error(), ref.MarshalString()) {
		t.Fatalf("upload = %v, want ErrRefsUnknown naming %s", err, ref.MarshalString())
	}
	found, err := remote.GetBlockExists(ctx, ref)
	if err != nil || found {
		t.Fatalf("remote = %v/%v, want absent", found, err)
	}
	if status, _ := s.GetStatus(); status.Pending != 1 || !errors.Is(status.Err, block.ErrRefsUnknown) {
		t.Fatalf("status = %+v, want one pending block and ErrRefsUnknown", status)
	}
}

// TestMarkingStoreStageHoldsCompleteRoots checks that a stage of the marking
// store holds completion proofs under the inner stage.
func TestMarkingStoreStageHoldsCompleteRoots(t *testing.T) {
	block_store_testing.RequireStageHold(t, func(ops block.StoreOps) block.StoreOps {
		return NewMarkingStore(ops, nil)
	})
}
