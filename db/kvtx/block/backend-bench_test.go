package kvtx_block

import (
	"context"
	"encoding/binary"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_traverse "github.com/s4wave/spacewave/db/block/traverse"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/hash"
)

type benchBlockStore struct {
	inner block.StoreOps

	getBlocks        atomic.Int64
	getBytes         atomic.Int64
	existsBlocks     atomic.Int64
	existsBatchCalls atomic.Int64
	existsBatchRefs  atomic.Int64
	putBlocks        atomic.Int64
	putBytes         atomic.Int64
	putRefs          atomic.Int64
	putBatchCalls    atomic.Int64
	putBatchEntries  atomic.Int64
	putBatchMax      atomic.Int64
	rmBlocks         atomic.Int64
	statBlocks       atomic.Int64
	syncCalls        atomic.Int64
}

func newBenchBlockStore() *benchBlockStore {
	return &benchBlockStore{inner: block_mock.NewMockStore(0)}
}

func (s *benchBlockStore) GetHashType() hash.HashType {
	return s.inner.GetHashType()
}

func (s *benchBlockStore) GetSupportedFeatures() block.StoreFeature {
	return s.inner.GetSupportedFeatures()
}

func (s *benchBlockStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

func (s *benchBlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, found, err := s.inner.PutBlock(ctx, data, opts)
	if err == nil {
		s.putBlocks.Add(1)
		s.putBytes.Add(int64(len(data)))
		if opts != nil {
			s.putRefs.Add(int64(len(opts.Refs)))
		}
	}
	return ref, found, err
}

func (s *benchBlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	err := s.inner.PutBlockBatch(ctx, entries)
	if err != nil {
		return err
	}
	s.putBatchCalls.Add(1)
	s.putBatchEntries.Add(int64(len(entries)))
	recordBenchMax(&s.putBatchMax, int64(len(entries)))
	for _, entry := range entries {
		if entry.Tombstone {
			continue
		}
		s.putBlocks.Add(1)
		s.putBytes.Add(int64(len(entry.Data)))
		s.putRefs.Add(int64(len(entry.Refs)))
	}
	return nil
}

func (s *benchBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := s.inner.GetBlock(ctx, ref)
	if err == nil {
		s.getBlocks.Add(1)
		if found {
			s.getBytes.Add(int64(len(data)))
		}
	}
	return data, found, err
}

func (s *benchBlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	found, err := s.inner.GetBlockExists(ctx, ref)
	if err == nil {
		s.existsBlocks.Add(1)
	}
	return found, err
}

func (s *benchBlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	found, err := s.inner.GetBlockExistsBatch(ctx, refs)
	if err == nil {
		s.existsBatchCalls.Add(1)
		s.existsBatchRefs.Add(int64(len(refs)))
	}
	return found, err
}

func (s *benchBlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	err := s.inner.RmBlock(ctx, ref)
	if err == nil {
		s.rmBlocks.Add(1)
	}
	return err
}

func (s *benchBlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	stat, err := s.inner.StatBlock(ctx, ref)
	if err == nil {
		s.statBlocks.Add(1)
	}
	return stat, err
}

func (s *benchBlockStore) Sync(ctx context.Context) (bool, error) {
	fenced, err := s.inner.Sync(ctx)
	if err == nil {
		s.syncCalls.Add(1)
	}
	return fenced, err
}

func (s *benchBlockStore) BeginDeferFlush() {
	block.BeginDeferFlush(s.inner)
}

func (s *benchBlockStore) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, s.inner)
}

func (s *benchBlockStore) resetCounts() {
	s.getBlocks.Store(0)
	s.getBytes.Store(0)
	s.existsBlocks.Store(0)
	s.existsBatchCalls.Store(0)
	s.existsBatchRefs.Store(0)
	s.putBlocks.Store(0)
	s.putBytes.Store(0)
	s.putRefs.Store(0)
	s.putBatchCalls.Store(0)
	s.putBatchEntries.Store(0)
	s.putBatchMax.Store(0)
	s.rmBlocks.Store(0)
	s.statBlocks.Store(0)
	s.syncCalls.Store(0)
}

func (s *benchBlockStore) reportMetrics(b *testing.B, ops int64) {
	if ops == 0 {
		return
	}
	denom := float64(ops)
	b.ReportMetric(float64(s.getBlocks.Load())/denom, "get-blocks/op")
	b.ReportMetric(float64(s.getBytes.Load())/denom, "get-bytes/op")
	b.ReportMetric(float64(s.existsBlocks.Load())/denom, "exists-blocks/op")
	b.ReportMetric(float64(s.existsBatchCalls.Load())/denom, "exists-batches/op")
	b.ReportMetric(float64(s.existsBatchRefs.Load())/denom, "exists-batch-refs/op")
	b.ReportMetric(float64(s.putBlocks.Load())/denom, "put-blocks/op")
	b.ReportMetric(float64(s.putBytes.Load())/denom, "put-bytes/op")
	b.ReportMetric(float64(s.putRefs.Load())/denom, "put-refs/op")
	b.ReportMetric(float64(s.putBatchCalls.Load())/denom, "put-batches/op")
	b.ReportMetric(float64(s.putBatchEntries.Load())/denom, "put-batch-entries/op")
	b.ReportMetric(float64(s.putBatchMax.Load()), "put-batch-max")
	b.ReportMetric(float64(s.rmBlocks.Load())/denom, "rm-blocks/op")
	b.ReportMetric(float64(s.statBlocks.Load())/denom, "stat-blocks/op")
	b.ReportMetric(float64(s.syncCalls.Load())/denom, "syncs/op")
}

func recordBenchMax(target *atomic.Int64, next int64) {
	for {
		current := target.Load()
		if next <= current {
			return
		}
		if target.CompareAndSwap(current, next) {
			return
		}
	}
}

type benchRefGraph struct {
	addRefs      atomic.Int64
	removeRefs   atomic.Int64
	applyBatches atomic.Int64
}

func (g *benchRefGraph) AddRef(context.Context, string, string) error {
	g.addRefs.Add(1)
	return nil
}

func (g *benchRefGraph) RemoveRef(context.Context, string, string) error {
	g.removeRefs.Add(1)
	return nil
}

func (g *benchRefGraph) ApplyRefBatch(_ context.Context, adds, removes []block_gc.RefEdge) error {
	g.applyBatches.Add(1)
	g.addRefs.Add(int64(len(adds)))
	g.removeRefs.Add(int64(len(removes)))
	return nil
}

func (g *benchRefGraph) RemoveNodeRefs(context.Context, string, bool) ([]string, error) {
	return nil, nil
}

func (g *benchRefGraph) HasIncomingRefs(context.Context, string) (bool, error) {
	return false, nil
}

func (g *benchRefGraph) HasIncomingRefsExcluding(context.Context, string, ...string) (bool, error) {
	return false, nil
}

func (g *benchRefGraph) GetOutgoingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (g *benchRefGraph) GetIncomingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (g *benchRefGraph) GetUnreferencedNodes(context.Context) ([]string, error) {
	return nil, nil
}

func (g *benchRefGraph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	return g.AddRef(ctx, block_gc.BlockIRI(source), block_gc.BlockIRI(target))
}

func (g *benchRefGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	return g.AddRef(ctx, objectKey, block_gc.BlockIRI(ref))
}

func (g *benchRefGraph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	return g.RemoveRef(ctx, objectKey, block_gc.BlockIRI(ref))
}

func (g *benchRefGraph) Close() error {
	return nil
}

func (g *benchRefGraph) resetCounts() {
	g.addRefs.Store(0)
	g.removeRefs.Store(0)
	g.applyBatches.Store(0)
}

func (g *benchRefGraph) reportMetrics(b *testing.B, ops int64) {
	if ops == 0 {
		return
	}
	denom := float64(ops)
	b.ReportMetric(float64(g.addRefs.Load())/denom, "gc-add-refs/op")
	b.ReportMetric(float64(g.removeRefs.Load())/denom, "gc-remove-refs/op")
	b.ReportMetric(float64(g.applyBatches.Load())/denom, "gc-apply-batches/op")
}

type benchKVTree struct {
	impl    KVImplType
	store   *benchBlockStore
	ops     block.StoreOps
	rootRef *block.BlockRef
	keys    [][]byte
}

type benchRootMetrics struct {
	height    uint32
	dagBlocks int
	dagBytes  int64
	maxDepth  int
}

type benchKeyKind string

const (
	benchKeySequential benchKeyKind = "sequential"
	benchKeyGraph      benchKeyKind = "graph"

	benchGraphGroupSize = 64
)

func TestKVTXBackendBenchHarness(t *testing.T) {
	ctx := context.Background()
	for _, impl := range benchKVImpls() {
		t.Run(impl.String(), func(t *testing.T) {
			tree := buildBenchKVTree(t, impl, makeBenchKeys(64, benchKeyGraph), false, false)
			tx := newBenchKVReadTx(t, ctx, tree)
			defer tx.Discard()

			_, found, err := tx.Get(ctx, tree.keys[17])
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("key not found")
			}
			var count int
			err = tx.ScanPrefixKeys(ctx, benchGraphPrefix(0), func([]byte) error {
				count++
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if count != benchGraphGroupSize {
				t.Fatalf("prefix count = %d, want %d", count, benchGraphGroupSize)
			}
			metrics := measureBenchKVRoot(t, ctx, tree)
			if metrics.height == 0 || metrics.dagBlocks == 0 || metrics.dagBytes == 0 {
				t.Fatalf("invalid root metrics: %#v", metrics)
			}
		})
	}
}

func BenchmarkKVTXBackendGetCursorAtKey(b *testing.B) {
	for _, size := range []int{16, 1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "cold", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					tx := newBenchKVReadTx(b, ctx, tree)
					cursor, err := tx.GetCursorAtKey(ctx, tree.keys[benchLookupIndex(i, size)])
					tx.Discard()
					if err != nil {
						b.Fatal(err)
					}
					if cursor == nil {
						b.Fatal("key not found")
					}
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
			b.Run(benchKVName(impl, "warm", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, false)
				tx := newBenchKVReadTx(b, ctx, tree)
				defer tx.Discard()
				for _, key := range tree.keys {
					if _, err := tx.GetCursorAtKey(ctx, key); err != nil {
						b.Fatal(err)
					}
				}
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					cursor, err := tx.GetCursorAtKey(ctx, tree.keys[benchLookupIndex(i, size)])
					if err != nil {
						b.Fatal(err)
					}
					if cursor == nil {
						b.Fatal("key not found")
					}
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendGetValue(b *testing.B) {
	for _, size := range []int{16, 1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "cold", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					tx := newBenchKVReadTx(b, ctx, tree)
					_, found, err := tx.Get(ctx, tree.keys[benchLookupIndex(i, size)])
					tx.Discard()
					if err != nil {
						b.Fatal(err)
					}
					if !found {
						b.Fatal("key not found")
					}
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
			b.Run(benchKVName(impl, "warm", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, false)
				tx := newBenchKVReadTx(b, ctx, tree)
				defer tx.Discard()
				for _, key := range tree.keys {
					_, found, err := tx.Get(ctx, key)
					if err != nil {
						b.Fatal(err)
					}
					if !found {
						b.Fatal("key not found")
					}
				}
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					_, found, err := tx.Get(ctx, tree.keys[benchLookupIndex(i, size)])
					if err != nil {
						b.Fatal(err)
					}
					if !found {
						b.Fatal("key not found")
					}
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendTinyMetadataUpdateCommit(b *testing.B) {
	for _, size := range []int{16} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "tiny-metadata-updates_4", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					runBenchKVUpdates(b, ctx, tree, i, 4)
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendScanGraphPrefixKeys(b *testing.B) {
	for _, size := range []int{1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "graph-prefix", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeyGraph), false, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					tx := newBenchKVReadTx(b, ctx, tree)
					var count int
					err := tx.ScanPrefixKeys(ctx, benchGraphPrefix(benchLookupIndex(i, size)/benchGraphGroupSize), func([]byte) error {
						count++
						return nil
					})
					tx.Discard()
					if err != nil {
						b.Fatal(err)
					}
					if count == 0 {
						b.Fatal("expected prefix keys")
					}
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendScanGraphPrefixValues(b *testing.B) {
	for _, size := range []int{1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "graph-prefix", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeyGraph), false, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					tx := newBenchKVReadTx(b, ctx, tree)
					var count int
					err := tx.ScanPrefix(ctx, benchGraphPrefix(benchLookupIndex(i, size)/benchGraphGroupSize), func(_, _ []byte) error {
						count++
						return nil
					})
					tx.Discard()
					if err != nil {
						b.Fatal(err)
					}
					if count == 0 {
						b.Fatal("expected prefix values")
					}
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendIndexedLogNextIndex(b *testing.B) {
	for _, size := range []int{1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "indexed-log", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeIndexedLogBenchKeys(size), true, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					tx := newBenchKVReadTx(b, ctx, tree)
					next, err := NextIndexedLogIndex(ctx, tx)
					tx.Discard()
					if err != nil {
						b.Fatal(err)
					}
					if next != uint64(size) {
						b.Fatalf("next index = %d, want %d", next, size)
					}
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendIndexedLogAppend(b *testing.B) {
	for _, size := range []int{1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "indexed-log-append", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeIndexedLogBenchKeys(size), true, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for range b.N {
					runBenchKVIndexedLogAppend(b, ctx, tree)
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendCursorValueRead(b *testing.B) {
	for _, size := range []int{1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "cursor-values", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, true)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					tx := newBenchKVReadTx(b, ctx, tree)
					cursor, err := tx.GetCursorAtKey(ctx, tree.keys[benchLookupIndex(i, size)])
					tx.Discard()
					if err != nil {
						b.Fatal(err)
					}
					if cursor == nil {
						b.Fatal("key not found")
					}
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendUpdateCommit(b *testing.B) {
	for _, size := range []int{1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "updates_16", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					runBenchKVUpdates(b, ctx, tree, i, 16)
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendDeleteCommit(b *testing.B) {
	for _, size := range []int{1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "pure-deletes_16", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					runBenchKVDeletes(b, ctx, tree, i, 16)
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
			b.Run(benchKVName(impl, "delete-reinsert_16", size), func(b *testing.B) {
				ctx := context.Background()
				tree := buildBenchKVTree(b, impl, makeBenchKeys(size, benchKeySequential), false, false)
				tree.store.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					runBenchKVDeleteReinsert(b, ctx, tree, i, 16)
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func BenchmarkKVTXBackendUpdateCommitGC(b *testing.B) {
	for _, size := range []int{1024} {
		for _, impl := range benchKVImpls() {
			b.Run(benchKVName(impl, "updates_16", size), func(b *testing.B) {
				ctx := context.Background()
				tree, refGraph := buildBenchKVTreeWithGC(b, impl, makeBenchKeys(size, benchKeySequential))
				tree.store.resetCounts()
				refGraph.resetCounts()
				b.ReportAllocs()
				b.ResetTimer()
				for i := range b.N {
					runBenchKVUpdates(b, ctx, tree, i, 16)
				}
				b.StopTimer()
				tree.store.reportMetrics(b, int64(b.N))
				refGraph.reportMetrics(b, int64(b.N))
				reportBenchKVRoot(b, ctx, tree)
			})
		}
	}
}

func benchKVImpls() []KVImplType {
	return []KVImplType{
		KVImplType_KV_IMPL_TYPE_IAVL,
		KVImplType_KV_IMPL_TYPE_OKRA,
	}
}

func buildBenchKVTree(
	tb testing.TB,
	impl KVImplType,
	keys [][]byte,
	indexedLog bool,
	cursorValues bool,
) *benchKVTree {
	tb.Helper()

	store := newBenchBlockStore()
	return buildBenchKVTreeWithOps(tb, impl, keys, store, store, indexedLog, cursorValues, nil)
}

func buildBenchKVTreeWithGC(tb testing.TB, impl KVImplType, keys [][]byte) (*benchKVTree, *benchRefGraph) {
	tb.Helper()

	store := newBenchBlockStore()
	refGraph := &benchRefGraph{}
	gcStore := block_gc.NewGCStoreOps(store, refGraph)
	tree := buildBenchKVTreeWithOps(tb, impl, keys, store, gcStore, false, false, gcStore.FlushPending)
	refGraph.resetCounts()
	return tree, refGraph
}

func buildBenchKVTreeWithOps(
	tb testing.TB,
	impl KVImplType,
	keys [][]byte,
	countStore *benchBlockStore,
	ops block.StoreOps,
	indexedLog bool,
	cursorValues bool,
	afterBuild func(context.Context) error,
) *benchKVTree {
	tb.Helper()

	ctx := context.Background()
	btx, rootCursor := block.NewTransaction(ops, nil, nil, nil)
	rootCursor.SetBlock(NewKeyValueStore(impl), true)
	tx, err := BuildKvTransaction(ctx, rootCursor, true)
	if err != nil {
		tb.Fatal(err)
	}
	for i, key := range keys {
		if cursorValues {
			ref, _, err := ops.PutBlock(ctx, benchValue(i), nil)
			if err != nil {
				tx.Discard()
				tb.Fatal(err)
			}
			_, valueCursor := block.NewTransaction(ops, nil, ref, nil)
			if err := tx.SetCursorAtKey(ctx, key, valueCursor, false); err != nil {
				tx.Discard()
				tb.Fatal(err)
			}
		} else {
			valueIndex := i
			if indexedLog {
				valueIndex += len(keys)
			}
			if err := tx.Set(ctx, key, benchValue(valueIndex)); err != nil {
				tx.Discard()
				tb.Fatal(err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		tb.Fatal(err)
	}
	tx.Discard()
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	if afterBuild != nil {
		if err := afterBuild(ctx); err != nil {
			tb.Fatal(err)
		}
	}
	countStore.resetCounts()

	return &benchKVTree{
		impl:    impl,
		store:   countStore,
		ops:     ops,
		rootRef: rootRef,
		keys:    keys,
	}
}

func (t *benchKVTree) storeOps() block.StoreOps {
	if t.ops != nil {
		return t.ops
	}
	return t.store
}

func newBenchKVReadTx(tb testing.TB, ctx context.Context, tree *benchKVTree) kvtx.BlockTx {
	tb.Helper()

	_, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
	tx, err := BuildKvTransaction(ctx, rootCursor, false)
	if err != nil {
		tb.Fatal(err)
	}
	return tx
}

func newBenchKVWriteTx(tb testing.TB, ctx context.Context, tree *benchKVTree) (*block.Transaction, kvtx.BlockTx) {
	tb.Helper()

	btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
	tx, err := BuildKvTransaction(ctx, rootCursor, true)
	if err != nil {
		tb.Fatal(err)
	}
	return btx, tx
}

func runBenchKVUpdates(tb testing.TB, ctx context.Context, tree *benchKVTree, offset, count int) {
	tb.Helper()

	btx, tx := newBenchKVWriteTx(tb, ctx, tree)
	for updateIndex := range count {
		key := tree.keys[benchLookupIndex(offset+updateIndex, len(tree.keys))]
		if err := tx.Set(ctx, key, benchValue(offset+updateIndex+len(tree.keys))); err != nil {
			tx.Discard()
			tb.Fatal(err)
		}
	}
	commitBenchKVTx(tb, ctx, btx, tx)
}

func runBenchKVDeletes(tb testing.TB, ctx context.Context, tree *benchKVTree, offset, count int) {
	tb.Helper()

	btx, tx := newBenchKVWriteTx(tb, ctx, tree)
	for deleteIndex := range count {
		key := tree.keys[benchLookupIndex(offset+deleteIndex, len(tree.keys))]
		if err := tx.Delete(ctx, key); err != nil {
			tx.Discard()
			tb.Fatal(err)
		}
	}
	commitBenchKVTx(tb, ctx, btx, tx)
}

func runBenchKVDeleteReinsert(tb testing.TB, ctx context.Context, tree *benchKVTree, offset, count int) {
	tb.Helper()

	btx, tx := newBenchKVWriteTx(tb, ctx, tree)
	for deleteIndex := range count {
		key := tree.keys[benchLookupIndex(offset+deleteIndex, len(tree.keys))]
		if err := tx.Delete(ctx, key); err != nil {
			tx.Discard()
			tb.Fatal(err)
		}
		if err := tx.Set(ctx, key, benchValue(offset+deleteIndex+len(tree.keys)*2)); err != nil {
			tx.Discard()
			tb.Fatal(err)
		}
	}
	commitBenchKVTx(tb, ctx, btx, tx)
}

func runBenchKVIndexedLogAppend(tb testing.TB, ctx context.Context, tree *benchKVTree) {
	tb.Helper()

	btx, tx := newBenchKVWriteTx(tb, ctx, tree)
	next, err := NextIndexedLogIndex(ctx, tx)
	if err != nil {
		tx.Discard()
		tb.Fatal(err)
	}
	if err := tx.Set(ctx, IndexedLogKey(next), benchValue(int(next))); err != nil {
		tx.Discard()
		tb.Fatal(err)
	}
	commitBenchKVTx(tb, ctx, btx, tx)
}

func commitBenchKVTx(tb testing.TB, ctx context.Context, btx *block.Transaction, tx kvtx.BlockTx) {
	tb.Helper()

	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		tb.Fatal(err)
	}
	tx.Discard()
	if _, _, err := btx.Write(ctx, true); err != nil {
		tb.Fatal(err)
	}
}

func measureBenchKVRoot(tb testing.TB, ctx context.Context, tree *benchKVTree) benchRootMetrics {
	tb.Helper()

	_, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
	root, err := LoadKeyValueStore(ctx, rootCursor)
	if err != nil {
		tb.Fatal(err)
	}
	metrics := benchRootMetrics{}
	switch tree.impl {
	case KVImplType_KV_IMPL_TYPE_IAVL:
		metrics.height = root.GetIavlRoot().GetHeight()
	case KVImplType_KV_IMPL_TYPE_OKRA:
		metrics.height = root.GetOkraRoot().GetHeight()
	}
	err = block_traverse.Visit(ctx, root, rootCursor, func(loc *block_traverse.Location) error {
		if loc.Depth > metrics.maxDepth {
			metrics.maxDepth = loc.Depth
		}
		if loc.Cursor.IsSubBlock() || loc.Cursor.GetRef().GetEmpty() {
			return nil
		}
		metrics.dagBlocks++
		stat, err := tree.storeOps().StatBlock(ctx, loc.Cursor.GetRef())
		if err != nil {
			return err
		}
		if stat != nil && stat.Size >= 0 {
			metrics.dagBytes += stat.Size
		}
		return nil
	}, false)
	if err != nil {
		tb.Fatal(err)
	}
	return metrics
}

func reportBenchKVRoot(b *testing.B, ctx context.Context, tree *benchKVTree) {
	b.Helper()

	metrics := measureBenchKVRoot(b, ctx, tree)
	b.ReportMetric(float64(metrics.height), "tree-height")
	b.ReportMetric(float64(metrics.dagBlocks), "root-dag-blocks")
	b.ReportMetric(float64(metrics.dagBytes), "root-dag-bytes")
	b.ReportMetric(float64(metrics.maxDepth), "root-dag-max-depth")
}

func makeBenchKeys(size int, kind benchKeyKind) [][]byte {
	keys := make([][]byte, size)
	for i := range size {
		switch kind {
		case benchKeyGraph:
			keys[i] = makeGraphBenchKey(i)
		default:
			keys[i] = makeSequentialBenchKey(i)
		}
	}
	return keys
}

func makeIndexedLogBenchKeys(size int) [][]byte {
	keys := make([][]byte, size)
	for i := range size {
		keys[i] = IndexedLogKey(uint64(i))
	}
	return keys
}

func makeSequentialBenchKey(i int) []byte {
	key := make([]byte, 8)
	binary.BigEndian.PutUint64(key, uint64(i))
	return key
}

func makeGraphBenchKey(i int) []byte {
	key := make([]byte, 16)
	binary.BigEndian.PutUint32(key[0:4], uint32(i/benchGraphGroupSize))
	binary.BigEndian.PutUint32(key[4:8], uint32(i%benchGraphGroupSize))
	binary.BigEndian.PutUint64(key[8:16], uint64(i)*0x9e3779b185ebca87)
	return key
}

func benchGraphPrefix(group int) []byte {
	prefix := make([]byte, 4)
	binary.BigEndian.PutUint32(prefix, uint32(group))
	return prefix
}

func benchValue(i int) []byte {
	value := make([]byte, 32)
	binary.BigEndian.PutUint64(value[0:8], uint64(i))
	binary.BigEndian.PutUint64(value[8:16], uint64(i*3+1))
	binary.BigEndian.PutUint64(value[16:24], uint64(i*5+2))
	binary.BigEndian.PutUint64(value[24:32], uint64(i*7+3))
	return value
}

func benchLookupIndex(i, size int) int {
	return (i * 8191) % size
}

func benchKVName(impl KVImplType, workload string, size int) string {
	return impl.String() + "/" + workload + "/keys_" + strconv.Itoa(size)
}
