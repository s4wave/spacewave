package kvtx_block_okra

import (
	"bytes"
	"context"
	"fmt"
	"math/rand/v2"
	"runtime"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

// liveHeap returns the heap in use after a full collection.
func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// newPaddedFixture stores size values under zero-padded keys, so the
// fixture's order is the tree's order at any size.
func newPaddedFixture(t testing.TB, ctx context.Context, store block.StoreOps, size int) *okraFixture {
	t.Helper()
	out := &okraFixture{keys: make([][]byte, size), values: make([][]byte, size), refs: make([]*block.BlockRef, size)}
	for i := range size {
		out.keys[i] = fmt.Appendf(nil, "key-%08d", i)
		out.values[i] = fmt.Appendf(nil, "value-%d", i)
		ref, _, err := store.PutBlock(ctx, out.values[i], nil)
		if err != nil {
			t.Fatal(err)
		}
		out.refs[i] = ref
	}
	return out
}

// storeFixture writes the fixture's tree and returns its root reference.
func storeFixture(t testing.TB, ctx context.Context, store block.StoreOps, fixture *okraFixture) *block.BlockRef {
	t.Helper()
	tx, _, err := BuildTree(ctx, store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return rootRef
}

// openReadTx opens a read transaction on the stored root through a decoded
// cache of cacheBytes, or with no cache scope when cacheBytes is zero.
func openReadTx(t testing.TB, ctx context.Context, store block.StoreOps, rootRef *block.BlockRef, cacheBytes int64) *Tx {
	t.Helper()
	bt, cs := block.NewTransaction(store, nil, rootRef, nil)
	bt.SetReadOnly()
	if cacheBytes != 0 {
		cache, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{MaxCost: cacheBytes})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(cache.Close)
		bt.SetDecodedBlockCache(cache)
	}
	tr, err := NewTx(ctx, cs, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tr.Discard)
	return tr
}

// TestReadWalkHeapFlat checks that a read transaction's walk over a tree far
// larger than its decoded cache keeps only the path it is on: the heap after
// the walk matches the heap before it. With no cache scope the transaction
// reads through its own scope in the process-wide pool, so the walk runs with
// that pool's budget at 1 MiB and must stay within it.
func TestReadWalkHeapFlat(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a large tree")
	}
	ctx := t.Context()
	store := newOkraTestStore()
	fixture := newPaddedFixture(t, ctx, store, 200_000)
	rootRef := storeFixture(t, ctx, store, fixture)
	for _, tc := range []struct {
		name       string
		cacheBytes int64
	}{{"cached", 1 << 20}, {"scopeless", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.cacheBytes == 0 {
				shared := block.NewDecodedBlockCache().MaxCost()
				block.SetSharedDecodedBlockCacheMaxCost(1 << 20)
				t.Cleanup(func() { block.SetSharedDecodedBlockCacheMaxCost(shared) })
			}
			walkHeapFlat(t, openReadTx(t, ctx, store, rootRef, tc.cacheBytes), fixture)
		})
	}
}

// walkHeapFlat walks every key of tr, checking the order against fixture and
// that the live heap does not grow with the walk.
func walkHeapFlat(t *testing.T, tr *Tx, fixture *okraFixture) {
	ctx := t.Context()

	// Walk every key, sampling the heap at the start, middle and end.
	it := tr.Iterate(ctx, nil, true, false)
	defer it.Close()
	start := liveHeap()
	var mid uint64
	n := 0
	for it.Next() {
		if !bytes.Equal(it.Key(), fixture.keys[n]) {
			t.Fatalf("key %d: got %q", n, it.Key())
		}
		n++
		if n == len(fixture.keys)/2 {
			mid = liveHeap()
		}
	}
	if err := it.Err(); err != nil {
		t.Fatal(err)
	}
	end := liveHeap()
	t.Logf("walked %d keys: heap %.1f MiB at start, %.1f MiB mid, %.1f MiB at end", n, mib(start), mib(mid), mib(end))
	if n != len(fixture.keys) {
		t.Fatalf("walked %d of %d keys", n, len(fixture.keys))
	}
	if grown := int64(end) - int64(start); grown > 4<<20 {
		t.Fatalf("walk kept %.1f MiB", float64(grown)/(1<<20))
	}
}

// TestReadWalkKeepsStagedPages checks that a read over a tree whose pages are
// staged but not stored still reads every key after a walk: a staged child
// lives only on its parent's edge, so the walk must keep that edge.
func TestReadWalkKeepsStagedPages(t *testing.T) {
	ctx := t.Context()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 4096)
	_, rootCursor, err := BuildTree(ctx, store, nil, nil, fixture.seq())
	if err != nil {
		t.Fatal(err)
	}
	tr, err := NewTx(ctx, rootCursor, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tr.Discard()

	// Walk twice, then look every key up.
	for pass := range 2 {
		it := tr.Iterate(ctx, nil, true, false)
		n := 0
		for it.Next() {
			n++
		}
		err := it.Err()
		it.Close()
		if err != nil || n != len(fixture.keys) {
			t.Fatalf("pass %d: walked %d of %d keys: %v", pass, n, len(fixture.keys), err)
		}
	}
	values, found, err := tr.GetBatch(ctx, fixture.keys)
	if err != nil {
		t.Fatal(err)
	}
	for i := range fixture.keys {
		if !found[i] || !bytes.Equal(values[i], fixture.values[i]) {
			t.Fatalf("key %q: found=%v value %q", fixture.keys[i], found[i], values[i])
		}
	}
}

// BenchmarkReadTxLookups measures repeated single-key lookups in one read
// transaction, within one key region and scattered over the tree, through
// GetBatch and through Get, with the default decoded cache and with no cache
// scope.
func BenchmarkReadTxLookups(b *testing.B) {
	ctx := b.Context()
	store := newOkraTestStore()
	fixture := newPaddedFixture(b, ctx, store, 50_000)
	rootRef := storeFixture(b, ctx, store, fixture)
	lookups := []struct {
		name string
		get  func(tr *Tx, key []byte) (bool, error)
	}{
		{"batch", func(tr *Tx, key []byte) (bool, error) {
			_, found, err := tr.GetBatch(ctx, [][]byte{key})
			return err == nil && found[0], err
		}},
		{"get", func(tr *Tx, key []byte) (bool, error) {
			_, found, err := tr.Get(ctx, key)
			return found, err
		}},
	}
	caches := []struct {
		name  string
		bytes int64
	}{{"", block.DefaultDecodedBlockCacheMaxCost}, {"-uncached", 0}}
	for _, cache := range caches {
		for _, bc := range []struct {
			name string
			span int
		}{{"region", 256}, {"scattered", len(fixture.keys)}} {
			for _, lookup := range lookups {
				b.Run(bc.name+"-"+lookup.name+cache.name, func(b *testing.B) {
					tr := openReadTx(b, ctx, store, rootRef, cache.bytes)
					rng := rand.New(rand.NewPCG(1, 2))
					base := len(fixture.keys) / 3
					b.ReportAllocs()
					for b.Loop() {
						key := fixture.keys[(base+rng.IntN(bc.span))%len(fixture.keys)]
						if found, err := lookup.get(tr, key); err != nil || !found {
							b.Fatalf("key %q: found=%v err=%v", key, found, err)
						}
					}
				})
			}
		}
	}
}

// BenchmarkReadTxBatches measures multi-key GetBatch calls in one read
// transaction with the default decoded cache: batches of 16 and 256 keys,
// within one key region and scattered over the tree. ns/key is the time per
// requested key.
func BenchmarkReadTxBatches(b *testing.B) {
	// Store a 50,000-key tree to read from.
	ctx := b.Context()
	store := newOkraTestStore()
	fixture := newPaddedFixture(b, ctx, store, 50_000)
	rootRef := storeFixture(b, ctx, store, fixture)

	// Run each batch size within one region and scattered over the tree.
	for _, size := range []int{16, 256} {
		for _, bc := range []struct {
			name string
			span int
		}{{"region", 1024}, {"scattered", len(fixture.keys)}} {
			b.Run(fmt.Sprintf("%s-%d", bc.name, size), func(b *testing.B) {
				// Draw each batch's keys from the span starting a third of the way in.
				tr := openReadTx(b, ctx, store, rootRef, block.DefaultDecodedBlockCacheMaxCost)
				rng := rand.New(rand.NewPCG(1, 2))
				base := len(fixture.keys) / 3
				keys := make([][]byte, size)
				b.ReportAllocs()
				for b.Loop() {
					for i := range keys {
						keys[i] = fixture.keys[(base+rng.IntN(bc.span))%len(fixture.keys)]
					}
					_, found, err := tr.GetBatch(ctx, keys)
					if err != nil || !found[0] {
						b.Fatalf("batch: found=%v err=%v", found[0], err)
					}
				}
				b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*size), "ns/key")
			})
		}
	}
}

// mib returns n bytes in MiB.
func mib(n uint64) float64 {
	return float64(n) / (1 << 20)
}
