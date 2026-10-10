package block_store_kvtx

import (
	"bytes"
	"context"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

func TestBeginReadOperationReusesReadTransaction(t *testing.T) {
	// Create a block store that counts underlying read transactions.
	ctx := context.Background()
	kvkey := store_kvkey.NewDefaultKVKey()
	store := &countingStore{inner: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	blocks := NewKVTxBlock(kvkey, store, 0, false)

	// Store a new block for repeated read checks.
	ref, existed, err := blocks.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Fatal("new block unexpectedly existed")
	}

	// Verify independent block reads open separate transactions.
	store.reads.Store(0)
	if _, found, err := blocks.GetBlock(ctx, ref); err != nil || !found {
		t.Fatalf("first get found=%v err=%v", found, err)
	}
	if _, found, err := blocks.GetBlock(ctx, ref); err != nil || !found {
		t.Fatalf("second get found=%v err=%v", found, err)
	}
	if got := store.reads.Load(); got != 2 {
		t.Fatalf("regular gets opened %d read transactions, want 2", got)
	}

	// Open a shared read scope with a fresh transaction count.
	store.reads.Store(0)
	scoped, release, err := blocks.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Verify repeated scoped reads reuse one transaction.
	if _, found, err := scoped.GetBlock(ctx, ref); err != nil || !found {
		t.Fatalf("scoped first get found=%v err=%v", found, err)
	}
	if _, found, err := scoped.GetBlock(ctx, ref); err != nil || !found {
		t.Fatalf("scoped second get found=%v err=%v", found, err)
	}
	if got := store.reads.Load(); got != 1 {
		t.Fatalf("scoped gets opened %d read transactions, want 1", got)
	}

	// Release the read scope and verify further reads fail.
	release()
	if _, _, err := scoped.GetBlock(ctx, ref); err != ErrReadOperationClosed {
		t.Fatalf("get after release err=%v, want %v", err, ErrReadOperationClosed)
	}
}

func TestPutBlockBatchUsesSingleWriteTransaction(t *testing.T) {
	// Create a counting block store and two batch fixtures.
	ctx := context.Background()
	store := newCountingStore()
	blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)
	firstData := []byte("first batch block")
	secondData := []byte("second batch block")
	firstRef := mustBuildBlockRef(t, firstData)
	secondRef := mustBuildBlockRef(t, secondData)

	// Write both blocks in one batch.
	store.reset()
	existed, err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: firstRef, Data: firstData},
		{Ref: secondRef, Data: secondData},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(existed, []bool{false, false}) {
		t.Fatalf("first batch existed = %v, want both new", existed)
	}

	// Verify the batch commits both blocks in one transaction.
	if got := store.writes.Load(); got != 1 {
		t.Fatalf("batch opened %d write transactions, want 1", got)
	}
	if got := store.commits.Load(); got != 1 {
		t.Fatalf("batch committed %d transactions, want 1", got)
	}
	if got := store.sets.Load(); got != 2 {
		t.Fatalf("batch set %d keys, want 2", got)
	}
	if data, found, err := blocks.GetBlock(ctx, firstRef); err != nil || !found || string(data) != string(firstData) {
		t.Fatalf("first get data=%q found=%v err=%v", data, found, err)
	}
	if data, found, err := blocks.GetBlock(ctx, secondRef); err != nil || !found || string(data) != string(secondData) {
		t.Fatalf("second get data=%q found=%v err=%v", data, found, err)
	}

	// Write the same batch again after resetting transaction counts.
	store.reset()
	existed, err = blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: firstRef, Data: firstData},
		{Ref: secondRef, Data: secondData},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(existed, []bool{true, true}) {
		t.Fatalf("repeated batch existed = %v, want both existing", existed)
	}

	// Verify the repeated batch commits without rewriting block keys.
	if got := store.writes.Load(); got != 1 {
		t.Fatalf("existing batch opened %d write transactions, want 1", got)
	}
	if got := store.commits.Load(); got != 1 {
		t.Fatalf("existing batch committed %d transactions, want 1", got)
	}
	if got := store.sets.Load(); got != 0 {
		t.Fatalf("existing batch set %d keys, want 0", got)
	}
}

func TestPutBlockBatchTombstoneUsesSameWriteTransaction(t *testing.T) {
	// Create a counting block store for a mixed write and tombstone batch.
	ctx := context.Background()
	store := newCountingStore()
	blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)

	// Prepare old, retained, and new block fixtures.
	oldData := []byte("old batch block")
	keepData := []byte("keep batch block")
	newData := []byte("new batch block")
	oldRef := mustBuildBlockRef(t, oldData)
	keepRef := mustBuildBlockRef(t, keepData)
	newRef := mustBuildBlockRef(t, newData)

	// Store the original blocks before applying the tombstone.
	if _, err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: oldRef, Data: oldData},
		{Ref: keepRef, Data: keepData},
	}); err != nil {
		t.Fatal(err)
	}

	// Write a batch that deletes the old block and stores the new block.
	store.reset()
	if _, err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: oldRef, Tombstone: true},
		{Ref: newRef, Data: newData},
	}); err != nil {
		t.Fatal(err)
	}

	// Verify the mixed batch commits its write and deletion once.
	if got := store.writes.Load(); got != 1 {
		t.Fatalf("mixed batch opened %d write transactions, want 1", got)
	}
	if got := store.commits.Load(); got != 1 {
		t.Fatalf("mixed batch committed %d transactions, want 1", got)
	}
	if got := store.sets.Load(); got != 1 {
		t.Fatalf("mixed batch set %d keys, want 1", got)
	}
	if got := store.deletes.Load(); got != 1 {
		t.Fatalf("mixed batch deleted %d keys, want 1", got)
	}

	// Verify the mixed batch preserves exactly the intended blocks.
	if found, err := blocks.GetBlockExists(ctx, oldRef); err != nil || found {
		t.Fatalf("old ref found=%v err=%v, want absent", found, err)
	}
	if found, err := blocks.GetBlockExists(ctx, keepRef); err != nil || !found {
		t.Fatalf("keep ref found=%v err=%v, want present", found, err)
	}
	if found, err := blocks.GetBlockExists(ctx, newRef); err != nil || !found {
		t.Fatalf("new ref found=%v err=%v, want present", found, err)
	}
}

func TestPutBlockBatchValidationErrorsDoNotCommitPartialBatch(t *testing.T) {
	// Create a counting block store with valid and mismatched references.
	ctx := context.Background()
	store := newCountingStore()
	blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)
	goodData := []byte("good batch block")
	goodRef := mustBuildBlockRef(t, goodData)
	wrongRef := mustBuildBlockRef(t, []byte("different batch block"))

	// Write a batch containing a mismatched block reference.
	_, err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: goodRef, Data: goodData},
		{Ref: wrongRef, Data: []byte("bad batch block")},
	})

	// Verify reference validation rejects the batch before any write.
	if err != block.ErrBlockRefMismatch {
		t.Fatalf("mismatch err=%v, want %v", err, block.ErrBlockRefMismatch)
	}
	if got := store.writes.Load(); got != 0 {
		t.Fatalf("mismatch batch opened %d write transactions, want 0", got)
	}
	if got := store.commits.Load(); got != 0 {
		t.Fatalf("mismatch batch committed %d transactions, want 0", got)
	}
	if got := store.sets.Load(); got != 0 {
		t.Fatalf("mismatch batch set %d keys, want 0", got)
	}
	if found, err := blocks.GetBlockExists(ctx, goodRef); err != nil || found {
		t.Fatalf("good ref found=%v err=%v after mismatch, want absent", found, err)
	}

	// Write a batch containing empty block data.
	store.reset()
	_, err = blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: goodRef, Data: goodData},
		{Data: []byte{}},
	})

	// Verify empty-block validation rejects the batch before any write.
	if err != block.ErrEmptyBlock {
		t.Fatalf("empty err=%v, want %v", err, block.ErrEmptyBlock)
	}
	if got := store.writes.Load(); got != 0 {
		t.Fatalf("empty batch opened %d write transactions, want 0", got)
	}
	if got := store.commits.Load(); got != 0 {
		t.Fatalf("empty batch committed %d transactions, want 0", got)
	}
	if got := store.sets.Load(); got != 0 {
		t.Fatalf("empty batch set %d keys, want 0", got)
	}
	if found, err := blocks.GetBlockExists(ctx, goodRef); err != nil || found {
		t.Fatalf("good ref found=%v err=%v after empty data, want absent", found, err)
	}
}

func TestGetBlockExistsBatchUsesSingleReadTransaction(t *testing.T) {
	// Create a counting block store for batched presence checks.
	ctx := context.Background()
	store := newCountingStore()
	blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)

	// Prepare stored and missing block references.
	firstData := []byte("exists first")
	secondData := []byte("exists second")
	missingData := []byte("exists missing")
	firstRef := mustBuildBlockRef(t, firstData)
	secondRef := mustBuildBlockRef(t, secondData)
	missingRef := mustBuildBlockRef(t, missingData)

	// Store the blocks that the presence batch should find.
	if _, err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: firstRef, Data: firstData},
		{Ref: secondRef, Data: secondData},
	}); err != nil {
		t.Fatal(err)
	}

	// Read the batch presence results with a fresh transaction count.
	store.reset()
	found, err := blocks.GetBlockExistsBatch(ctx, []*block.BlockRef{firstRef, missingRef, secondRef})
	if err != nil {
		t.Fatal(err)
	}

	// Verify one transaction returns the presence result for each reference.
	if got := store.reads.Load(); got != 1 {
		t.Fatalf("batch exists opened %d read transactions, want 1", got)
	}
	want := []bool{true, false, true}
	for i, got := range found {
		if got != want[i] {
			t.Fatalf("found[%d]=%v, want %v", i, got, want[i])
		}
	}
}

func TestPutBlockBatchRetriesWholeLogicalOperation(t *testing.T) {
	// Define a batch write experiment with optional commit failure.
	type result struct {
		first  []byte
		second []byte
	}
	run := func(t *testing.T, injectFault bool) (result, *kvtest.FaultStore, *countingStore) {
		// Create the counting backend and optional commit fault injector.
		t.Helper()
		ctx := t.Context()
		backend := newCountingStore()
		var store kvtx.Store = backend
		var faultStore *kvtest.FaultStore
		if injectFault {
			faultStore = kvtest.NewFaultStore(backend, kvtest.FaultBeforeCommit)
			store = faultStore
		}

		// Prepare the block store and batch fixtures for this run.
		blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)
		firstData := []byte("retry first batch block")
		secondData := []byte("retry second batch block")
		firstRef := mustBuildBlockRef(t, firstData)
		secondRef := mustBuildBlockRef(t, secondData)

		// Write both blocks through the optional commit fault injector.
		if _, err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
			{Ref: firstRef, Data: firstData},
			{Ref: secondRef, Data: secondData},
		}); err != nil {
			t.Fatal(err)
		}

		// Read the first block from the underlying committed store.
		reader := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), backend, 0, false)
		first, found, err := reader.GetBlock(ctx, firstRef)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the first committed block is present.
		if !found {
			t.Fatal("first committed block not found")
		}

		// Read the second block from the underlying committed store.
		second, found, err := reader.GetBlock(ctx, secondRef)
		if err != nil {
			t.Fatal(err)
		}

		// Verify the second committed block is present.
		if !found {
			t.Fatal("second committed block not found")
		}
		return result{first: first, second: second}, faultStore, backend
	}

	// Run the batch write with and without a commit failure.
	want, _, _ := run(t, false)
	got, faultStore, backend := run(t, true)

	// Verify replay preserves both block values and transaction cleanup.
	if !bytes.Equal(got.first, want.first) {
		t.Fatalf("first block = %q, want %q", got.first, want.first)
	}
	if !bytes.Equal(got.second, want.second) {
		t.Fatalf("second block = %q, want %q", got.second, want.second)
	}
	if got := faultStore.Opened(); got != 2 {
		t.Fatalf("opened transactions = %d, want 2", got)
	}
	if got := faultStore.Discarded(); got != 2 {
		t.Fatalf("discarded transactions = %d, want 2", got)
	}
	if got := faultStore.DelegatedCommits(); got != 1 {
		t.Fatalf("delegated commits = %d, want 1", got)
	}
	if got := backend.sets.Load(); got != 4 {
		t.Fatalf("batch set operations = %d, want 4 across two complete attempts", got)
	}
}

type countingStore struct {
	inner   kvtx.Store
	reads   atomic.Uint64
	writes  atomic.Uint64
	commits atomic.Uint64
	sets    atomic.Uint64
	deletes atomic.Uint64
}

func (c *countingStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		c.writes.Add(1)
	} else {
		c.reads.Add(1)
	}
	tx, err := c.inner.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &countingTx{Tx: tx, commits: &c.commits, sets: &c.sets, deletes: &c.deletes}, nil
}

func (c *countingStore) reset() {
	// Clear the store counters before measuring the next operation.
	c.reads.Store(0)
	c.writes.Store(0)
	c.commits.Store(0)
	c.sets.Store(0)
	c.deletes.Store(0)
}

type countingTx struct {
	kvtx.Tx
	commits *atomic.Uint64
	sets    *atomic.Uint64
	deletes *atomic.Uint64
}

func (c *countingTx) Commit(ctx context.Context) error {
	c.commits.Add(1)
	return c.Tx.Commit(ctx)
}

func (c *countingTx) Set(ctx context.Context, key, value []byte) error {
	c.sets.Add(1)
	return c.Tx.Set(ctx, key, value)
}

func (c *countingTx) Delete(ctx context.Context, key []byte) error {
	c.deletes.Add(1)
	return c.Tx.Delete(ctx, key)
}

func newCountingStore() *countingStore {
	return &countingStore{inner: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
}

func mustBuildBlockRef(t *testing.T, data []byte) *block.BlockRef {
	t.Helper()
	ref, err := block.BuildBlockRef(data, &block.PutOpts{})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// _ is a type assertion
var (
	_ kvtx.Store = (*countingStore)(nil)
	_ kvtx.Tx    = (*countingTx)(nil)
)
