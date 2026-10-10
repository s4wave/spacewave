package world_block

import (
	"context"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
)

type forkBufferRecordingStore struct {
	block.StoreOps

	mu           sync.Mutex
	batchCalls   int
	batchEntries int
	syncCalls    int
}

func (s *forkBufferRecordingStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Record the batch and delegate it to the store.
	s.mu.Lock()
	s.batchCalls++
	s.batchEntries += len(entries)
	s.mu.Unlock()
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

func (s *forkBufferRecordingStore) Sync(ctx context.Context) (bool, error) {
	s.mu.Lock()
	s.syncCalls++
	s.mu.Unlock()
	return s.StoreOps.Sync(ctx)
}

func (s *forkBufferRecordingStore) counts() (int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.batchCalls, s.batchEntries, s.syncCalls
}

func writeForkBufferTestBlock(
	ctx context.Context,
	t *testing.T,
	tx *Tx,
	message string,
) *block.BlockRef {
	// Attribute nested block write failures to the caller.
	t.Helper()

	// Write the example block through the World state.
	var ref *block.BlockRef
	err := tx.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		// Write the example block and return its ref.
		btx, bcs := cursor.BuildTransaction(nil)
		bcs.SetBlock(block_mock.NewExample(message), true)
		var err error
		ref, _, err = btx.Write(ctx, true)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func TestForkBlockTransactionBuffersNestedWritesUntilSync(t *testing.T) {
	// Open a retirement engine and record its write store.
	ctx := t.Context()
	engine := newRetirementTestEngine(t, ctx)
	recording := &forkBufferRecordingStore{StoreOps: engine.writeBlockStore}
	engine.writeBlockStore = recording

	// Fork a write transaction.
	tx, err := engine.ForkBlockTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Require the nested write to stay buffered.
	ref := writeForkBufferTestBlock(ctx, t, tx, "nested buffered block")
	if batches, _, _ := recording.counts(); batches != 0 {
		t.Fatalf("lower store batches before commit: got %d want 0", batches)
	}

	// Read the buffered block back from the transaction.
	if err := tx.AccessWorldState(ctx, &bucket.ObjectRef{RootRef: ref}, func(cursor *bucket_lookup.Cursor) error {
		_, bcs := cursor.BuildTransaction(nil)
		got, err := block_mock.UnmarshalExample(ctx, bcs)
		if err == nil && got.GetMsg() != "nested buffered block" {
			t.Fatalf("buffered block message: got %q", got.GetMsg())
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Commit the fork and require the write to stay buffered.
	if _, err := tx.CommitBlockTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	if batches, _, _ := recording.counts(); batches != 0 {
		t.Fatalf("lower store batches before Sync: got %d want 0", batches)
	}

	// Sync the fork and require one batch and one sync.
	if _, err := tx.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	batches, entries, syncs := recording.counts()
	if batches != 1 {
		t.Fatalf("lower store batches after Sync: got %d want 1", batches)
	}
	if entries == 0 {
		t.Fatal("lower store batch after Sync was empty")
	}
	if syncs != 1 {
		t.Fatalf("lower store Sync calls: got %d want 1", syncs)
	}
}

func TestForkBlockTransactionDiscardDropsNestedWrites(t *testing.T) {
	// Open a retirement engine and record its write store.
	ctx := t.Context()
	engine := newRetirementTestEngine(t, ctx)
	recording := &forkBufferRecordingStore{StoreOps: engine.writeBlockStore}
	engine.writeBlockStore = recording

	// Fork a transaction, write a block, and discard it.
	tx, err := engine.ForkBlockTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	writeForkBufferTestBlock(ctx, t, tx, "discarded buffered block")
	tx.Discard()

	// Sync the engine and require the discarded write to be gone.
	if _, err := engine.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	batches, entries, _ := recording.counts()
	if batches != 0 || entries != 0 {
		t.Fatalf("discard published buffered writes: batches=%d entries=%d", batches, entries)
	}
}

func TestForkBlockTransactionFlushWritesWithoutSync(t *testing.T) {
	// Open a retirement engine and record its write store.
	ctx := t.Context()
	engine := newRetirementTestEngine(t, ctx)
	recording := &forkBufferRecordingStore{StoreOps: engine.writeBlockStore}
	engine.writeBlockStore = recording

	// Fork a write transaction.
	tx, err := engine.ForkBlockTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Flush the committed fork and require a batch without a sync.
	writeForkBufferTestBlock(ctx, t, tx, "flushed buffered block")
	if _, err := tx.CommitBlockTransaction(ctx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	batches, entries, syncs := recording.counts()
	if batches != 1 || entries == 0 {
		t.Fatalf("lower store writes after Flush: batches=%d entries=%d", batches, entries)
	}
	if syncs != 0 {
		t.Fatalf("lower store Sync calls after Flush: got %d want 0", syncs)
	}
}
