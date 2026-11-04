package kvtx_block_okra

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	blockstore "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/s4db"
	storekey "github.com/s4wave/spacewave/db/store/kvkey"
	memstore "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// diveCount matches the approximate World object count recorded downstream.
const diveCount = 300000

// diveKey returns a sorted, fixed-width World-shaped object key.
func diveKey(i int) []byte {
	// Pad the numeric suffix so numeric and lexical order agree.
	s := strconv.Itoa(i)
	key := []byte("object/000000000")
	copy(key[len(key)-len(s):], s)
	return key
}

// diveValue returns an independently checkable object-sized external value.
func diveValue(i int, changed bool) []byte {
	// Keep the object key in the body so wrong-key reads cannot pass validation.
	value := append(diveKey(i), bytes.Repeat([]byte("v"), 96)...)
	if changed {
		value[len(value)-1] = 'w'
	}
	return value
}

// diveFixture owns an immutable file and both revisions for one experiment.
type diveFixture struct {
	// db retains the file throughout every read and graph lifetime.
	db *s4db.DB
	// store serves committed file blocks with the existing admission contract.
	store block.StoreOps
	// roots contains the original and sparsely changed Okra metadata.
	roots [2]*Root
}

// openDiveFixture builds a disposable fixture once, then reopens its real file.
func openDiveFixture(tb testing.TB) *diveFixture {
	// Create the fixture directory, refusing to use a live store path.
	tb.Helper()
	dir := os.Getenv("DIVE_DIR")
	if dir == "" {
		tb.Fatal("DIVE_DIR must select disposable experiment data")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		tb.Fatal(err)
	}
	path := filepath.Join(dir, "fixture.s4wave")
	if _, err := os.Stat(filepath.Join(dir, "new-root.pb")); os.IsNotExist(err) {
		buildDiveFixture(tb, dir, path)
	}

	// Open the immutable measurement store without compaction or other writers.
	db, err := s4db.Open(path, s4db.Options{SingleProcess: true, RelocateBudget: -1})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := db.Close(); err != nil {
			tb.Error(err)
		}
	})
	f := &diveFixture{db: db, store: blockstore.NewDurableKVTxBlock(storekey.NewDefaultKVKey(), db, 0, false)}
	for i, name := range []string{"old-root.pb", "new-root.pb"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			tb.Fatal(err)
		}
		f.roots[i] = &Root{}
		if err := f.roots[i].UnmarshalVT(data); err != nil {
			tb.Fatal(err)
		}
	}
	return f
}

// buildDiveFixture streams a production-shaped tree into a real s4db file.
func buildDiveFixture(tb testing.TB, dir, path string) {
	// Build sorted external-value entries using the real Okra builder.
	tb.Helper()
	ctx := context.Background()
	mem := memstore.NewStore()
	store := blockstore.NewDurableKVTxBlock(storekey.NewDefaultKVKey(), mem, 0, false)
	var buildErr error
	tx, cursor, err := BuildTree(ctx, store, nil, nil, func(yield func([]byte, *block.BlockRef) bool) {
		for i := range diveCount {
			ref, _, err := store.PutBlock(ctx, diveValue(i, false), nil)
			if err != nil {
				buildErr = err
				return
			}
			if !yield(diveKey(i), ref) {
				return
			}
		}
	})
	if err != nil {
		tb.Fatal(err)
	}
	if buildErr != nil {
		tb.Fatal(buildErr)
	}
	ref, _, err := tx.Write(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	oldRoot, err := block.UnmarshalBlock[Root](ctx, cursor)
	if err != nil {
		tb.Fatal(err)
	}
	oldData, err := oldRoot.MarshalVT()
	if err != nil {
		tb.Fatal(err)
	}

	// Replace 74 spread-out values and insert a key that shifts a leaf slot.
	btx, bcs := block.NewTransaction(store, nil, ref, nil)
	writer, err := NewTx(ctx, bcs, btx, true, nil)
	if err != nil {
		tb.Fatal(err)
	}
	for i := 0; i < diveCount; i += 4096 {
		if err := writer.Set(ctx, diveKey(i), diveValue(i, true)); err != nil {
			tb.Fatal(err)
		}
	}
	if err := writer.Set(ctx, []byte("object/000000000-extra"), []byte("inserted")); err != nil {
		tb.Fatal(err)
	}
	if err := writer.Commit(ctx); err != nil {
		tb.Fatal(err)
	}
	newRoot, err := block.UnmarshalBlock[Root](ctx, bcs)
	if err != nil {
		tb.Fatal(err)
	}
	newData, err := newRoot.MarshalVT()
	if err != nil {
		tb.Fatal(err)
	}

	// Stream committed blocks into one disposable physical database transaction.
	db, err := s4db.Open(path, s4db.Options{SingleProcess: true, RelocateBudget: -1})
	if err != nil {
		tb.Fatal(err)
	}
	physical, err := db.NewTransaction(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	source, err := mem.NewTransaction(ctx, false)
	if err != nil {
		tb.Fatal(err)
	}
	err = source.ScanPrefix(ctx, nil, func(k, v []byte) error { return physical.Set(ctx, k, v) })
	source.Discard()
	if err != nil {
		physical.Discard()
		tb.Fatal(err)
	}
	if err := physical.Commit(ctx); err != nil {
		tb.Fatal(err)
	}
	if err := db.Close(); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "old-root.pb"), oldData, 0o600); err != nil {
		tb.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new-root.pb"), newData, 0o600); err != nil {
		tb.Fatal(err)
	}
}

// diveNode retains an immutable page and lazy child associations, never parents.
type diveNode struct {
	// page is immutable throughout the pinned fixture lifetime.
	page *Page
	// children memoizes only page edges visited by a read.
	children []*diveNode
}

// diveGraph is a native experiment scoped to one immutable retained file.
// It deliberately omits general storage removal and eviction and cannot ship.
type diveGraph struct {
	// mutex serializes lazy graph fills for a conservative initial comparison.
	mutex sync.Mutex
	// store belongs to the fixture and outlives all nodes.
	store block.StoreOps
	// cache preserves the existing decode path on node misses.
	cache *block.DecodedBlockCache
	// nodes retains page identities within this single store and transform scope.
	nodes map[string]*diveNode
	// cost estimates page and edge heap retained by the experiment.
	cost int
}

// newDiveGraph constructs the single-store prototype with the baseline decoder.
func newDiveGraph(store block.StoreOps, cache *block.DecodedBlockCache) *diveGraph {
	return &diveGraph{store: store, cache: cache, nodes: make(map[string]*diveNode)}
}

// load returns a node by full block reference, without consulting an old root.
func (g *diveGraph) load(ctx context.Context, ref *block.BlockRef) (*diveNode, error) {
	// Reuse a known immutable page within this retained file.
	key := ref.MarshalString()
	if node := g.nodes[key]; node != nil {
		return node, nil
	}

	// Load and validate missing pages through the existing block cache machinery.
	tx, cs := block.NewTransaction(g.store, nil, ref, nil)
	tx.SetReadOnly()
	tx.SetDecodedBlockCache(g.cache)
	page, err := loadPage(ctx, cs)
	if err != nil {
		return nil, err
	}
	node := &diveNode{page: page}
	if page.GetLevel() != 0 {
		node.children = make([]*diveNode, len(page.GetEntries()))
	}
	cost := page.DecodedHeapSize() + len(node.children)*8 + len(key) + 128
	if g.cost+cost > 64<<20 {
		return nil, errors.New("prototype retained graph exceeds 64 MiB experiment ceiling")
	}
	g.nodes[key] = node
	g.cost += cost
	return node, nil
}

// locate resolves one key through retained immutable pages.
func (g *diveGraph) locate(ctx context.Context, node *diveNode, key []byte) (*Page, int, error) {
	// Keep fills safe while the experimental graph is shared across revisions.
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	g.mutex.Lock()
	defer g.mutex.Unlock()
	var err error

	// Descend through memoized immutable edges, filling missing children lazily.
	for node.page.GetLevel() != 0 {
		idx := node.page.searchEntry(key)
		if idx < 0 {
			return nil, 0, nil
		}
		child := node.children[idx]
		if child == nil {
			child, err = g.load(ctx, node.page.GetEntries()[idx].GetChildRef())
			if err != nil {
				return nil, 0, err
			}
			node.children[idx] = child
		}
		if !child.page.containsKey(key) {
			return nil, 0, nil
		}
		node = child
	}

	// Require an exact key match before exposing a leaf entry.
	idx := node.page.searchEntry(key)
	if idx < 0 {
		return nil, 0, nil
	}
	ent := node.page.GetEntries()[idx]
	if ent.GetAnchor() || !bytes.Equal(ent.GetKey(), key) {
		return nil, 0, nil
	}
	return node.page, idx, nil
}

// read performs the same external or blob value fetch as Tx.Get.
func (g *diveGraph) read(ctx context.Context, root *diveNode, key []byte) ([]byte, bool, error) {
	// Resolve membership while preserving the page throughout this read.
	page, idx, err := g.locate(ctx, root, key)
	if err != nil || page == nil {
		return nil, false, err
	}
	ent := page.GetEntries()[idx]

	// Fetch the owned value bytes after releasing the graph fill mutex.
	if raw, ok := page.rawInlineValue(idx); ok {
		return bytes.Clone(raw), true, ctx.Err()
	}
	_, cs := block.NewTransaction(g.store, nil, ent.GetValueRef(), nil)
	if ent.GetValueIsBlob() {
		value, err := blob.FetchToBytes(ctx, cs)
		return value, true, err
	}
	value, _, err := cs.Fetch(ctx)
	return value, true, err
}

// diveRead runs one validated transaction-sized batch on either implementation.
func diveRead(tb testing.TB, f *diveFixture, cache *block.DecodedBlockCache, graph *diveGraph, ctx context.Context, rootIndex int, keys [][]byte, values bool) {
	// Open the baseline with decoded metadata, as a World sub-block would be seeded.
	tb.Helper()
	var current *Tx
	var rootNode *diveNode
	if graph != nil {
		graph.mutex.Lock()
		var err error
		rootNode, err = graph.load(ctx, f.roots[rootIndex].GetRootPageRef())
		graph.mutex.Unlock()
		if err != nil {
			tb.Fatal(err)
		}
	}
	if graph == nil {
		tx, cs := block.NewTransaction(f.store, nil, nil, nil)
		tx.SetReadOnly()
		tx.SetDecodedBlockCache(cache)
		cs.SetBlock(f.roots[rootIndex].CloneVT(), false)
		var err error
		current, err = NewTx(ctx, cs, nil, false, nil)
		if err != nil {
			tb.Fatal(err)
		}
		defer current.Discard()
	}

	// Read and verify every requested byte, including changed and held old roots.
	for _, key := range keys {
		var value []byte
		var found bool
		var err error
		switch {
		case graph != nil && values:
			value, found, err = graph.read(ctx, rootNode, key)
		case graph != nil:
			var page *Page
			page, _, err = graph.locate(ctx, rootNode, key)
			found = page != nil
		case values:
			value, found, err = current.Get(ctx, key)
		default:
			found, err = current.Exists(ctx, key)
		}
		if err != nil {
			tb.Fatal(err)
		}
		if !found {
			tb.Fatalf("missing key %q at root %d", key, rootIndex)
		}
		if !values {
			continue
		}
		index, err := strconv.Atoi(string(key[len("object/"):]))
		if err != nil {
			tb.Fatal(err)
		}
		if !found || !bytes.Equal(value, diveValue(index, rootIndex == 1 && index%4096 == 0)) {
			tb.Fatalf("wrong value for %q at root %d", key, rootIndex)
		}
	}
}

// diveKeys returns a deterministic spread or sequential held-out workload.
func diveKeys(n int, sequential bool) [][]byte {
	keys := make([][]byte, n)
	for i := range keys {
		index := (i * 7919) % diveCount
		if sequential {
			index = 100000 + i
		}
		keys[i] = diveKey(index)
	}
	return keys
}

// newDiveCache creates an independent baseline-sized decoded pool.
func newDiveCache(tb testing.TB) *block.DecodedBlockCache {
	tb.Helper()
	cache, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{MaxCost: 256 << 20})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(cache.Close)
	return cache
}

// BenchmarkLazyHashDive compares equivalent file-backed reads after warmup.
func BenchmarkLazyHashDive(b *testing.B) {
	// Keep the same immutable fixture and query sets for every alternating side.
	f := openDiveFixture(b)
	for _, values := range []bool{true, false} {
		kind := "exists"
		if values {
			kind = "get"
		}
		for _, n := range []int{1, 64, 1024} {
			keys := diveKeys(n, false)
			for _, side := range []string{"baseline", "lazy"} {
				b.Run(kind+"/"+strconv.Itoa(n)+"/"+side, func(b *testing.B) {
					// Warm both revisions outside timing while checking all returned values.
					cache := newDiveCache(b)
					var graph *diveGraph
					if side == "lazy" {
						graph = newDiveGraph(f.store, cache)
					}
					diveRead(b, f, cache, graph, context.Background(), 0, keys, values)
					diveRead(b, f, cache, graph, context.Background(), 1, keys, values)
					b.ReportAllocs()
					b.ResetTimer()
					i := 0
					for b.Loop() {
						diveRead(b, f, cache, graph, context.Background(), i%2, keys, values)
						i++
					}
				})
			}
		}
	}
}

// TestLazyHashDive records cold, post-HEAD, held-out and memory diagnostics.
func TestLazyHashDive(t *testing.T) {
	// Measure each side at an empty graph, after warmup, and after a sparse HEAD edit.
	f := openDiveFixture(t)
	keys := diveKeys(1024, false)
	for sample := range 5 {
		for _, side := range []string{"baseline", "lazy"} {
			cache := newDiveCache(t)
			var graph *diveGraph
			if side == "lazy" {
				graph = newDiveGraph(f.store, cache)
			}
			for _, phase := range []string{"cold", "warm", "head", "old-snapshot", "held-out"} {
				root := 0
				useKeys := keys
				if phase == "head" {
					root = 1
				}
				if phase == "held-out" {
					useKeys = diveKeys(1024, true)
				}
				ctx, counter := block.WithReadCounter(context.Background())
				start := time.Now()
				diveRead(t, f, cache, graph, ctx, root, useKeys, true)
				elapsed := time.Since(start)
				counts := counter.Snapshot()
				t.Logf("sample=%d side=%s phase=%s ns=%d reads=%d decode=%d hits=%d clones=%d", sample, side, phase, elapsed.Nanoseconds(), counts.BlockReadCount, counts.DecodedBlockUnmarshalCount, counts.DecodedBlockCacheHitCount, counts.DecodedBlockCloneCount)
			}

			// Report graph-resident bytes separately from the decoded pool's budget.
			runtime.GC()
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			snap := cache.Snapshot()
			cost, nodes := 0, 0
			if graph != nil {
				cost, nodes = graph.cost, len(graph.nodes)
			}
			t.Logf("sample=%d side=%s nodes=%d graph_cost=%d pool_cost=%d heap=%d", sample, side, nodes, cost, snap.RetainedCost, stats.HeapAlloc)
			runtime.KeepAlive(graph)
			cache.Close()
		}
	}
}
