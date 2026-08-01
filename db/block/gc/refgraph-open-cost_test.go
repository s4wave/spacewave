package block_gc

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestNewRefGraphOpenCostIsIndependentOfEdgeCount pins the property that makes
// a RefGraph usable on a large store: opening one reads a fixed number of keys
// no matter how many gc/ref edges the graph holds. A RefGraph that materializes
// its edges at open turns every process start into a full scan of the store,
// and the store is meant to hold arbitrary amounts of data.
func TestNewRefGraphOpenCostIsIndependentOfEdgeCount(t *testing.T) {
	ctx := context.Background()
	small := openCostForEdgeCount(t, ctx, 16)
	large := openCostForEdgeCount(t, ctx, 2048)
	if small != large {
		t.Fatalf(
			"opening a graph of 16 edges read %d keys and one of 2048 edges read %d; open scales with edge count",
			small, large,
		)
	}
}

// openCostForEdgeCount seeds edgeCount gc/ref edges into a fresh store, then
// reports how many key reads a second RefGraph over that same store performs
// while opening.
func openCostForEdgeCount(t *testing.T, ctx context.Context, edgeCount int) int64 {
	t.Helper()

	store := store_kvtx_inmem.NewStore()
	seed, err := NewRefGraph(ctx, store, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	edges := make([]RefEdge, edgeCount)
	for i := range edges {
		edges[i] = RefEdge{
			Subject: "open-cost/subject/" + strconv.Itoa(i),
			Object:  "open-cost/object/" + strconv.Itoa(i),
		}
	}
	if err := seed.ApplyRefBatch(ctx, edges, nil); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	counted := &countingStore{Store: store}
	rg, err := NewRefGraph(ctx, counted, []byte("gc/"))
	if err != nil {
		t.Fatal(err)
	}
	reads := counted.reads.Load()
	t.Cleanup(func() { _ = rg.Close() })
	return reads
}

// countingStore counts the keys read through every transaction it hands out.
type countingStore struct {
	kvtx.Store
	reads atomic.Int64
}

func (s *countingStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &countingTx{Tx: tx, store: s}, nil
}

type countingTx struct {
	kvtx.Tx
	store *countingStore
}

func (t *countingTx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	t.store.reads.Add(1)
	return t.Tx.Get(ctx, key)
}

func (t *countingTx) Exists(ctx context.Context, key []byte) (bool, error) {
	t.store.reads.Add(1)
	return t.Tx.Exists(ctx, key)
}

func (t *countingTx) ScanPrefix(ctx context.Context, prefix []byte, cb func(key, value []byte) error) error {
	return t.Tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		t.store.reads.Add(1)
		return cb(key, value)
	})
}

func (t *countingTx) ScanPrefixKeys(ctx context.Context, prefix []byte, cb func(key []byte) error) error {
	return t.Tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		t.store.reads.Add(1)
		return cb(key)
	})
}

func (t *countingTx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	return &countingIterator{
		Iterator: t.Tx.Iterate(ctx, prefix, sort, reverse),
		store:    t.store,
	}
}

// countingIterator charges one read per entry the caller steps onto, so a scan
// of the whole graph costs as much as reading every key by hand.
type countingIterator struct {
	kvtx.Iterator
	store *countingStore
}

func (it *countingIterator) Next() bool {
	if !it.Iterator.Next() {
		return false
	}
	it.store.reads.Add(1)
	return true
}
