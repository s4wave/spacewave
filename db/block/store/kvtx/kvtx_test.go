package block_store_kvtx

import (
	"context"
	"sync/atomic"
	"testing"

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

type countingStore struct {
	inner  kvtx.Store
	reads  atomic.Uint64
	writes atomic.Uint64
}

func (c *countingStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if write {
		c.writes.Add(1)
	} else {
		c.reads.Add(1)
	}
	return c.inner.NewTransaction(ctx, write)
}

// _ is a type assertion
var _ kvtx.Store = ((*countingStore)(nil))
