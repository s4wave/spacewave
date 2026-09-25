package block_store_writeback

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// failingStore rejects every batch write, standing in for an unreachable bucket.
type failingStore struct {
	block.StoreOps
}

// PutBlockBatch always fails.
func (f *failingStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return errors.New("bucket unreachable")
}

func newInmemBlockStore() block.StoreOps {
	return block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, false)
}

// TestUploadAfterRestart checks that queued writes survive a failed upload and
// a reopen, then drain to the remote store.
func TestUploadAfterRestart(t *testing.T) {
	ctx := t.Context()
	local := newInmemBlockStore()
	markers := store_kvtx_inmem.NewStore()

	// Unplaced writes are not queued.
	s, err := NewStore(ctx, local, markers, false)
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
	s, err = NewStore(ctx, local, markers, true)
	if err != nil {
		t.Fatal(err)
	}
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
			break
		}
		<-changed
	}
	uploadCancel()
	<-done

	for _, ref := range refs[:2] {
		found, err := remote.GetBlockExists(ctx, ref)
		if err != nil || !found {
			t.Fatalf("remote missing %s: %v", ref.MarshalString(), err)
		}
	}
}

// TestDisableDropsMarkers checks that disabling upload clears the queue.
func TestDisableDropsMarkers(t *testing.T) {
	ctx := t.Context()
	markers := store_kvtx_inmem.NewStore()
	s, err := NewStore(ctx, newInmemBlockStore(), markers, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PutBlock(ctx, []byte("queued"), nil); err != nil {
		t.Fatal(err)
	}
	if err := s.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := NewStore(ctx, newInmemBlockStore(), markers, true)
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := reopened.GetStatus(); status.Pending != 0 {
		t.Fatalf("pending %d after disable, expected 0", status.Pending)
	}
}
