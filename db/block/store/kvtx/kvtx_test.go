package block_store_kvtx

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
)

func TestBeginReadOperationReusesReadTransaction(t *testing.T) {
	ctx := context.Background()
	kvkey := store_kvkey.NewDefaultKVKey()
	store := &countingStore{inner: hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]())}
	blocks := NewKVTxBlock(kvkey, store, 0, false)

	ref, existed, err := blocks.PutBlock(ctx, []byte("hello"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if existed {
		t.Fatal("new block unexpectedly existed")
	}

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

	store.reads.Store(0)
	scoped, release, err := blocks.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if _, found, err := scoped.GetBlock(ctx, ref); err != nil || !found {
		t.Fatalf("scoped first get found=%v err=%v", found, err)
	}
	if _, found, err := scoped.GetBlock(ctx, ref); err != nil || !found {
		t.Fatalf("scoped second get found=%v err=%v", found, err)
	}
	if got := store.reads.Load(); got != 1 {
		t.Fatalf("scoped gets opened %d read transactions, want 1", got)
	}
	release()
	if _, _, err := scoped.GetBlock(ctx, ref); err != ErrReadOperationClosed {
		t.Fatalf("get after release err=%v, want %v", err, ErrReadOperationClosed)
	}
}

func TestPutBlockBatchUsesSingleWriteTransaction(t *testing.T) {
	ctx := context.Background()
	store := newCountingStore()
	blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)
	firstData := []byte("first batch block")
	secondData := []byte("second batch block")
	firstRef := mustBuildBlockRef(t, firstData)
	secondRef := mustBuildBlockRef(t, secondData)

	store.reset()
	if err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: firstRef, Data: firstData},
		{Ref: secondRef, Data: secondData},
	}); err != nil {
		t.Fatal(err)
	}
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

	store.reset()
	if err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: firstRef, Data: firstData},
		{Ref: secondRef, Data: secondData},
	}); err != nil {
		t.Fatal(err)
	}
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
	ctx := context.Background()
	store := newCountingStore()
	blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)
	oldData := []byte("old batch block")
	keepData := []byte("keep batch block")
	newData := []byte("new batch block")
	oldRef := mustBuildBlockRef(t, oldData)
	keepRef := mustBuildBlockRef(t, keepData)
	newRef := mustBuildBlockRef(t, newData)

	if err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: oldRef, Data: oldData},
		{Ref: keepRef, Data: keepData},
	}); err != nil {
		t.Fatal(err)
	}

	store.reset()
	if err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: oldRef, Tombstone: true},
		{Ref: newRef, Data: newData},
	}); err != nil {
		t.Fatal(err)
	}
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
	ctx := context.Background()
	store := newCountingStore()
	blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)
	goodData := []byte("good batch block")
	goodRef := mustBuildBlockRef(t, goodData)
	wrongRef := mustBuildBlockRef(t, []byte("different batch block"))

	err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: goodRef, Data: goodData},
		{Ref: wrongRef, Data: []byte("bad batch block")},
	})
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

	store.reset()
	err = blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: goodRef, Data: goodData},
		{Data: []byte{}},
	})
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
	ctx := context.Background()
	store := newCountingStore()
	blocks := NewKVTxBlock(store_kvkey.NewDefaultKVKey(), store, 0, false)
	firstData := []byte("exists first")
	secondData := []byte("exists second")
	missingData := []byte("exists missing")
	firstRef := mustBuildBlockRef(t, firstData)
	secondRef := mustBuildBlockRef(t, secondData)
	missingRef := mustBuildBlockRef(t, missingData)

	if err := blocks.PutBlockBatch(ctx, []*block.PutBatchEntry{
		{Ref: firstRef, Data: firstData},
		{Ref: secondRef, Data: secondData},
	}); err != nil {
		t.Fatal(err)
	}

	store.reset()
	found, err := blocks.GetBlockExistsBatch(ctx, []*block.BlockRef{firstRef, missingRef, secondRef})
	if err != nil {
		t.Fatal(err)
	}
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
	_ kvtx.Store = ((*countingStore)(nil))
	_ kvtx.Tx    = ((*countingTx)(nil))
)
