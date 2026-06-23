package kvtx_block_iavl

import (
	"context"
	"encoding/binary"
	"iter"
	"os"
	"path/filepath"
	runtimetrace "runtime/trace"
	"strconv"
	"sync/atomic"
	"testing"

	badger "github.com/dgraph-io/badger/v4"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_badger "github.com/s4wave/spacewave/db/store/kvtx/badger"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/net/hash"
)

type benchBlockStore struct {
	// inner is the measured block store.
	inner block.StoreOps

	// getBlocks counts GetBlock calls.
	getBlocks atomic.Int64
	// getBytes counts bytes returned by GetBlock.
	getBytes atomic.Int64
	// existsBlocks counts GetBlockExists calls.
	existsBlocks atomic.Int64
	// existsBatchCalls counts GetBlockExistsBatch calls.
	existsBatchCalls atomic.Int64
	// existsBatchRefs counts refs passed to GetBlockExistsBatch.
	existsBatchRefs atomic.Int64
	// putBlocks counts PutBlock calls.
	putBlocks atomic.Int64
	// putBytes counts bytes passed to PutBlock.
	putBytes atomic.Int64
	// putRefs counts outgoing refs passed to PutBlock and PutBlockBatch.
	putRefs atomic.Int64
	// putBatchCalls counts PutBlockBatch calls.
	putBatchCalls atomic.Int64
	// putBatchEntries counts entries passed to PutBlockBatch.
	putBatchEntries atomic.Int64
	// putBatchMaxEntries records the largest PutBlockBatch entry count.
	putBatchMaxEntries atomic.Int64
	// rmBlocks counts RmBlock calls.
	rmBlocks atomic.Int64
	// statBlocks counts StatBlock calls.
	statBlocks atomic.Int64
	// syncCalls counts Sync calls.
	syncCalls atomic.Int64
}

func newBenchBlockStore() *benchBlockStore {
	return newBenchBlockStoreWithOps(block_mock.NewMockStore(0))
}

func newBenchBlockStoreWithOps(inner block.StoreOps) *benchBlockStore {
	return &benchBlockStore{
		inner: inner,
	}
}

func newBenchBadgerBlockStore(tb testing.TB) *benchBlockStore {
	tb.Helper()

	kv, err := store_kvtx_badger.Open(badger.DefaultOptions(tb.TempDir()).WithLogger(nil))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := kv.GetDB().Close(); err != nil {
			tb.Error(err)
		}
	})

	return newBenchBlockStoreWithOps(block_store_kvtx.NewKVTxBlock(
		store_kvkey.NewDefaultKVKey(),
		kv,
		0,
		false,
	))
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
	recordMaxInt64(&s.putBatchMaxEntries, int64(len(entries)))
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
	s.putBatchMaxEntries.Store(0)
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
	b.ReportMetric(float64(s.putBatchMaxEntries.Load()), "put-batch-max")
	b.ReportMetric(float64(s.rmBlocks.Load())/denom, "rm-blocks/op")
	b.ReportMetric(float64(s.statBlocks.Load())/denom, "stat-blocks/op")
	b.ReportMetric(float64(s.syncCalls.Load())/denom, "syncs/op")
}

func recordMaxInt64(target *atomic.Int64, next int64) {
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

type benchTree struct {
	// store is the counted block store.
	store *benchBlockStore
	// ops is the store used by IAVL transactions.
	ops block.StoreOps
	// rootRef is the persisted IAVL root block ref.
	rootRef *block.BlockRef
	// keys is the sorted key fixture.
	keys [][]byte
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

func buildBenchTree(tb testing.TB, size int) *benchTree {
	tb.Helper()

	return buildBenchTreeWithKeys(tb, makeBenchKeys(size, benchKeySequential))
}

func buildBenchTreeWithKeys(tb testing.TB, keys [][]byte) *benchTree {
	tb.Helper()

	return buildBenchTreeWithStore(tb, keys, newBenchBlockStore())
}

func buildBenchTreeWithStore(tb testing.TB, keys [][]byte, store *benchBlockStore) *benchTree {
	tb.Helper()

	return buildBenchTreeWithOps(tb, keys, store, store, nil)
}

func buildBenchTreeWithGC(tb testing.TB, keys [][]byte) (*benchTree, *benchRefGraph) {
	tb.Helper()

	store := newBenchBlockStore()
	refGraph := &benchRefGraph{}
	gcStore := block_gc.NewGCStoreOps(store, refGraph)
	tree := buildBenchTreeWithOps(tb, keys, store, gcStore, gcStore.FlushPending)
	refGraph.resetCounts()
	return tree, refGraph
}

func buildBenchTreeWithRealGC(tb testing.TB, keys [][]byte) *benchTree {
	tb.Helper()

	ctx := context.Background()
	kvStore := store_kvtx_inmem.NewStore()
	store := newBenchBlockStoreWithOps(block_store_kvtx.NewKVTxBlock(
		store_kvkey.NewDefaultKVKey(),
		kvStore,
		0,
		false,
	))
	refGraph, err := block_gc.NewRefGraph(ctx, kvStore, []byte("gc/"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := refGraph.Close(); err != nil {
			tb.Error(err)
		}
	})
	gcStore := block_gc.NewGCStoreOps(store, refGraph)
	return buildBenchTreeWithOps(tb, keys, store, gcStore, gcStore.FlushPending)
}

func buildBenchTreeWithOps(
	tb testing.TB,
	keys [][]byte,
	countStore *benchBlockStore,
	ops block.StoreOps,
	afterBuild func(context.Context) error,
) *benchTree {
	tb.Helper()

	ctx := context.Background()
	size := len(keys)
	refs := make([]*block.BlockRef, size)
	for i := range size {
		ref, _, err := ops.PutBlock(ctx, benchValue(i), nil)
		if err != nil {
			tb.Fatal(err)
		}
		refs[i] = ref
	}

	tx, _, err := BuildTree(ops, nil, nil, benchEntries(keys, refs))
	if err != nil {
		tb.Fatal(err)
	}
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	if afterBuild != nil {
		if err := afterBuild(ctx); err != nil {
			tb.Fatal(err)
		}
	}
	countStore.resetCounts()

	return &benchTree{
		store:   countStore,
		ops:     ops,
		rootRef: rootRef,
		keys:    keys,
	}
}

func (t *benchTree) storeOps() block.StoreOps {
	if t.ops != nil {
		return t.ops
	}
	return t.store
}

func newBenchReadTx(tb testing.TB, ctx context.Context, tree *benchTree) *Tx {
	tb.Helper()

	_, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
	tx, err := NewTx(ctx, rootCursor, nil, false, nil)
	if err != nil {
		tb.Fatal(err)
	}
	return tx
}

type benchKeyKind string

const (
	benchKeySequential benchKeyKind = "sequential"
	benchKeyGraph      benchKeyKind = "graph"

	benchGraphGroupSize = 128
)

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

func benchEntries(keys [][]byte, refs []*block.BlockRef) iter.Seq2[[]byte, *block.BlockRef] {
	return func(yield func([]byte, *block.BlockRef) bool) {
		for i, key := range keys {
			if !yield(key, refs[i]) {
				return
			}
		}
	}
}

func benchLookupIndex(i, size int) int {
	return (i * 8191) % size
}

func benchSizeName(size int) string {
	return "keys_" + strconv.Itoa(size)
}

func benchFixtureName(kind benchKeyKind, size int) string {
	return string(kind) + "/" + benchSizeName(size)
}

func TestIAVLBenchHarnessCounts(t *testing.T) {
	ctx := context.Background()
	tree := buildBenchTree(t, 32)
	tx := newBenchReadTx(t, ctx, tree)
	defer tx.Discard()

	_, err := tx.GetCursorAtKey(ctx, tree.keys[17])
	if err != nil {
		t.Fatal(err)
	}
	if tree.store.getBlocks.Load() == 0 {
		t.Fatal("expected counted block reads")
	}
}

func TestIAVLBenchHarnessGraphPrefix(t *testing.T) {
	ctx := context.Background()
	tree := buildBenchTreeWithKeys(t, makeBenchKeys(512, benchKeyGraph))
	tx := newBenchReadTx(t, ctx, tree)
	defer tx.Discard()

	var count int
	if err := tx.ScanPrefixKeys(ctx, benchGraphPrefix(2), func(key []byte) error {
		count++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if count != benchGraphGroupSize {
		t.Fatalf("expected %d graph prefix keys, got %d", benchGraphGroupSize, count)
	}
	if tree.store.getBlocks.Load() == 0 {
		t.Fatal("expected counted block reads")
	}
}

func TestIAVLDeleteAvoidsValueFetch(t *testing.T) {
	ctx := context.Background()
	deleteReads := measureBenchTreeDeleteReads(t, ctx, "delete")
	getAndDeleteReads := measureBenchTreeDeleteReads(t, ctx, "get-and-delete")

	if deleteReads >= getAndDeleteReads {
		t.Fatalf("Delete read %d blocks, GetAndDelete read %d blocks", deleteReads, getAndDeleteReads)
	}

	tree := buildBenchTree(t, 128)
	tx := newBenchReadTx(t, ctx, tree)
	if err := tx.Delete(ctx, makeSequentialBenchKey(len(tree.keys)+1)); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	tx.Discard()
}

func measureBenchTreeDeleteReads(t *testing.T, ctx context.Context, mode string) int64 {
	t.Helper()

	tree := buildBenchTree(t, 128)
	tree.store.resetCounts()
	tx := newBenchReadTx(t, ctx, tree)
	key := tree.keys[benchLookupIndex(7, len(tree.keys))]
	switch mode {
	case "delete":
		if err := tx.Delete(ctx, key); err != nil {
			tx.Discard()
			t.Fatal(err)
		}
	case "get-and-delete":
		if _, found, err := tx.GetAndDelete(ctx, key); err != nil {
			tx.Discard()
			t.Fatal(err)
		} else if !found {
			tx.Discard()
			t.Fatal("key not found")
		}
	default:
		t.Fatalf("unknown delete measurement mode %q", mode)
	}
	tx.Discard()
	return tree.store.getBlocks.Load()
}

func TestIAVLBenchBadgerBlockStoreCounts(t *testing.T) {
	ctx := context.Background()
	tree := buildBenchTreeWithStore(t, makeBenchKeys(32, benchKeySequential), newBenchBadgerBlockStore(t))
	tx := newBenchReadTx(t, ctx, tree)
	defer tx.Discard()

	_, found, err := tx.Get(ctx, tree.keys[17])
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("key not found")
	}
	if tree.store.getBlocks.Load() == 0 {
		t.Fatal("expected counted physical block reads")
	}
}

func TestIAVLBenchGCStoreCounts(t *testing.T) {
	ctx := context.Background()
	tree, refGraph := buildBenchTreeWithGC(t, makeBenchKeys(128, benchKeySequential))

	btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
	tx, err := NewTx(ctx, rootCursor, nil, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, tree.keys[17], benchValue(1000)); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	tx.Discard()
	if _, _, err := btx.Write(ctx, true); err != nil {
		t.Fatal(err)
	}

	if tree.store.existsBatchCalls.Load() == 0 {
		t.Fatal("expected GC commit to check existing blocks in a batch")
	}
	if refGraph.addRefs.Load() == 0 {
		t.Fatal("expected GC commit to flush ref graph additions")
	}
}

func TestIAVLSetRotationSequences(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		keys []int
	}{
		{name: "left_left", keys: []int{3, 2, 1}},
		{name: "right_right", keys: []int{1, 2, 3}},
		{name: "left_right", keys: []int{3, 1, 2}},
		{name: "right_left", keys: []int{1, 3, 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := newBenchBlockStore()
			_, rootCursor := block.NewTransaction(store, nil, nil, nil)
			rootCursor.SetBlock(&Node{}, true)
			tx, err := NewTx(ctx, rootCursor, nil, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Discard()

			for _, key := range tc.keys {
				if err := tx.Set(ctx, makeSequentialBenchKey(key), benchValue(key)); err != nil {
					t.Fatal(err)
				}
			}
			size, err := tx.Size(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if size != 3 {
				t.Fatalf("expected size 3, got %d", size)
			}
			if height := tx.Height(); height > 2 {
				t.Fatalf("expected height <= 2, got %d", height)
			}
			for key := 1; key <= 3; key++ {
				_, found, err := tx.Get(ctx, makeSequentialBenchKey(key))
				if err != nil {
					t.Fatal(err)
				}
				if !found {
					t.Fatalf("key %d not found", key)
				}
			}

			var prev []byte
			var count int
			if err := tx.ScanPrefixKeys(ctx, nil, func(key []byte) error {
				if prev != nil && string(prev) >= string(key) {
					t.Fatalf("keys out of order: %x >= %x", prev, key)
				}
				prev = append(prev[:0], key...)
				count++
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if count != 3 {
				t.Fatalf("expected 3 scanned keys, got %d", count)
			}
		})
	}
}

func runIAVLTrace(
	t *testing.T,
	name string,
	setup func(context.Context, *benchTree),
	body func(context.Context, *benchTree),
) {
	t.Helper()

	if os.Getenv("SPACEWAVE_IAVL_TRACE") != "1" {
		t.Skip("set SPACEWAVE_IAVL_TRACE=1 to capture IAVL runtime traces")
	}

	ctx := context.Background()
	tree := buildBenchTree(t, 16384)
	if setup != nil {
		setup(ctx, tree)
	}
	tracePath := filepath.Join("..", "..", "..", "..", ".tmp", "iavl-"+name+".trace")
	if err := os.MkdirAll(filepath.Dir(tracePath), 0o755); err != nil {
		t.Fatal(err)
	}
	traceFile, err := os.Create(tracePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimetrace.Start(traceFile); err != nil {
		if closeErr := traceFile.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal(err)
	}
	defer func() {
		runtimetrace.Stop()
		if err := traceFile.Close(); err != nil {
			t.Error(err)
		}
		t.Logf("trace: %s", tracePath)
	}()

	taskCtx, task := trace.NewTask(ctx, "iavl-bench/"+name)
	defer task.End()
	body(taskCtx, tree)
}

func TestIAVLTraceColdGetCursor(t *testing.T) {
	runIAVLTrace(t, "cold-get-cursor", nil, func(ctx context.Context, tree *benchTree) {
		tree.store.resetCounts()
		for i := range 128 {
			tx := newBenchReadTx(t, ctx, tree)
			cursor, err := tx.GetCursorAtKey(ctx, tree.keys[benchLookupIndex(i, len(tree.keys))])
			tx.Discard()
			if err != nil {
				t.Fatal(err)
			}
			if cursor == nil {
				t.Fatal("key not found")
			}
		}
		trace.Logf(
			ctx,
			"counts",
			"get-blocks=%d get-bytes=%d",
			tree.store.getBlocks.Load(),
			tree.store.getBytes.Load(),
		)
	})
}

func TestIAVLTraceWarmGetCursor(t *testing.T) {
	var warmTx *Tx
	runIAVLTrace(t, "warm-get-cursor", func(ctx context.Context, tree *benchTree) {
		warmTx = newBenchReadTx(t, ctx, tree)
		for _, key := range tree.keys {
			if _, err := warmTx.GetCursorAtKey(ctx, key); err != nil {
				t.Fatal(err)
			}
		}
	}, func(ctx context.Context, tree *benchTree) {
		defer warmTx.Discard()
		tree.store.resetCounts()
		for i := range 128 {
			cursor, err := warmTx.GetCursorAtKey(ctx, tree.keys[benchLookupIndex(i, len(tree.keys))])
			if err != nil {
				t.Fatal(err)
			}
			if cursor == nil {
				t.Fatal("key not found")
			}
		}
		trace.Logf(
			ctx,
			"counts",
			"get-blocks=%d get-bytes=%d",
			tree.store.getBlocks.Load(),
			tree.store.getBytes.Load(),
		)
	})
}

func TestIAVLTraceGetValue(t *testing.T) {
	runIAVLTrace(t, "get-value", nil, func(ctx context.Context, tree *benchTree) {
		tree.store.resetCounts()
		for i := range 128 {
			tx := newBenchReadTx(t, ctx, tree)
			_, found, err := tx.Get(ctx, tree.keys[benchLookupIndex(i, len(tree.keys))])
			tx.Discard()
			if err != nil {
				t.Fatal(err)
			}
			if !found {
				t.Fatal("key not found")
			}
		}
		trace.Logf(
			ctx,
			"counts",
			"get-blocks=%d get-bytes=%d",
			tree.store.getBlocks.Load(),
			tree.store.getBytes.Load(),
		)
	})
}

func TestIAVLTraceUpdateCommit(t *testing.T) {
	runIAVLTrace(t, "update-commit", nil, func(ctx context.Context, tree *benchTree) {
		tree.store.resetCounts()
		for i := range 10 {
			btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
			tx, err := NewTx(ctx, rootCursor, nil, true, nil)
			if err != nil {
				t.Fatal(err)
			}
			for updateIndex := range 100 {
				key := tree.keys[benchLookupIndex(i+updateIndex, len(tree.keys))]
				if err := tx.Set(ctx, key, benchValue(i+updateIndex+len(tree.keys))); err != nil {
					tx.Discard()
					t.Fatal(err)
				}
			}
			tx.Discard()
			if _, _, err := btx.Write(ctx, true); err != nil {
				t.Fatal(err)
			}
		}
		trace.Logf(
			ctx,
			"counts",
			"get-blocks=%d get-bytes=%d put-blocks=%d put-bytes=%d",
			tree.store.getBlocks.Load(),
			tree.store.getBytes.Load(),
			tree.store.putBlocks.Load(),
			tree.store.putBytes.Load(),
		)
	})
}

func BenchmarkIAVLGetCursorAtKey(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run("cold/"+benchSizeName(size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTree(b, size)
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				tx := newBenchReadTx(b, ctx, tree)
				_, err := tx.GetCursorAtKey(ctx, tree.keys[benchLookupIndex(i, size)])
				tx.Discard()
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
		})

		b.Run("warm/"+benchSizeName(size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTree(b, size)
			tx := newBenchReadTx(b, ctx, tree)
			defer tx.Discard()
			for _, key := range tree.keys {
				if _, err := tx.GetCursorAtKey(ctx, key); err != nil {
					b.Fatal(err)
				}
			}
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				_, err := tx.GetCursorAtKey(ctx, tree.keys[benchLookupIndex(i, size)])
				if err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
		})
	}
}

func BenchmarkIAVLGetValue(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run("cold/"+benchSizeName(size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTree(b, size)
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				tx := newBenchReadTx(b, ctx, tree)
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
		})
	}
}

func BenchmarkIAVLScanPrefixKeys(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run(benchFixtureName(benchKeyGraph, size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTreeWithKeys(b, makeBenchKeys(size, benchKeyGraph))
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				tx := newBenchReadTx(b, ctx, tree)
				var count int
				err := tx.ScanPrefixKeys(ctx, benchGraphPrefix(benchLookupIndex(i, size)/benchGraphGroupSize), func(key []byte) error {
					count++
					return nil
				})
				tx.Discard()
				if err != nil {
					b.Fatal(err)
				}
				if count == 0 {
					b.Fatal("expected matching prefix keys")
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
		})
	}
}

func BenchmarkIAVLScanPrefixValues(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run(benchFixtureName(benchKeyGraph, size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTreeWithKeys(b, makeBenchKeys(size, benchKeyGraph))
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				tx := newBenchReadTx(b, ctx, tree)
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
					b.Fatal("expected matching prefix values")
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
		})
	}
}

func BenchmarkIAVLUpdateCommit(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run("updates_100/"+benchSizeName(size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTree(b, size)
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
				tx, err := NewTx(ctx, rootCursor, nil, true, nil)
				if err != nil {
					b.Fatal(err)
				}
				for updateIndex := range 100 {
					key := tree.keys[benchLookupIndex(i+updateIndex, size)]
					if err := tx.Set(ctx, key, benchValue(i+updateIndex+size)); err != nil {
						b.Fatal(err)
					}
				}
				tx.Discard()
				if _, _, err := btx.Write(ctx, true); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
		})
	}
}

func BenchmarkIAVLUpdateCommitGC(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run("updates_100/"+benchSizeName(size), func(b *testing.B) {
			ctx := context.Background()
			tree, refGraph := buildBenchTreeWithGC(b, makeBenchKeys(size, benchKeySequential))
			tree.store.resetCounts()
			refGraph.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
				tx, err := NewTx(ctx, rootCursor, nil, true, nil)
				if err != nil {
					b.Fatal(err)
				}
				for updateIndex := range 100 {
					key := tree.keys[benchLookupIndex(i+updateIndex, size)]
					if err := tx.Set(ctx, key, benchValue(i+updateIndex+size)); err != nil {
						tx.Discard()
						b.Fatal(err)
					}
				}
				tx.Discard()
				if _, _, err := btx.Write(ctx, true); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
			refGraph.reportMetrics(b, int64(b.N))
		})
	}
}

func BenchmarkIAVLUpdateCommitGCRefGraph(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run("updates_100/"+benchSizeName(size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTreeWithRealGC(b, makeBenchKeys(size, benchKeySequential))
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
				tx, err := NewTx(ctx, rootCursor, nil, true, nil)
				if err != nil {
					b.Fatal(err)
				}
				for updateIndex := range 100 {
					key := tree.keys[benchLookupIndex(i+updateIndex, size)]
					if err := tx.Set(ctx, key, benchValue(i+updateIndex+size)); err != nil {
						tx.Discard()
						b.Fatal(err)
					}
				}
				tx.Discard()
				if _, _, err := btx.Write(ctx, true); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
		})
	}
}

func BenchmarkIAVLDeleteCommit(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run("deletes_100/"+benchSizeName(size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTree(b, size)
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
				tx, err := NewTx(ctx, rootCursor, nil, true, nil)
				if err != nil {
					b.Fatal(err)
				}
				for deleteIndex := range 100 {
					key := tree.keys[benchLookupIndex(i+deleteIndex, size)]
					if err := tx.Delete(ctx, key); err != nil {
						tx.Discard()
						b.Fatal(err)
					}
				}
				tx.Discard()
				if _, _, err := btx.Write(ctx, true); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
		})
	}
}

func BenchmarkIAVLDeleteCursorCommit(b *testing.B) {
	for _, size := range []int{1024, 16384} {
		b.Run("deletes_100/"+benchSizeName(size), func(b *testing.B) {
			ctx := context.Background()
			tree := buildBenchTree(b, size)
			tree.store.resetCounts()
			b.ResetTimer()
			for i := range b.N {
				btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
				tx, err := NewTx(ctx, rootCursor, nil, true, nil)
				if err != nil {
					b.Fatal(err)
				}
				for deleteIndex := range 100 {
					key := tree.keys[benchLookupIndex(i+deleteIndex, size)]
					if _, err := tx.DeleteCursorAtKey(ctx, key); err != nil {
						tx.Discard()
						b.Fatal(err)
					}
				}
				tx.Discard()
				if _, _, err := btx.Write(ctx, true); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			tree.store.reportMetrics(b, int64(b.N))
		})
	}
}

func BenchmarkIAVLBadgerBlockStore(b *testing.B) {
	const size = 1024

	b.Run("cold_get_cursor/"+benchSizeName(size), func(b *testing.B) {
		ctx := context.Background()
		tree := buildBenchTreeWithStore(b, makeBenchKeys(size, benchKeySequential), newBenchBadgerBlockStore(b))
		tree.store.resetCounts()
		b.ResetTimer()
		for i := range b.N {
			tx := newBenchReadTx(b, ctx, tree)
			_, err := tx.GetCursorAtKey(ctx, tree.keys[benchLookupIndex(i, size)])
			tx.Discard()
			if err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		tree.store.reportMetrics(b, int64(b.N))
	})

	b.Run("cold_get_value/"+benchSizeName(size), func(b *testing.B) {
		ctx := context.Background()
		tree := buildBenchTreeWithStore(b, makeBenchKeys(size, benchKeySequential), newBenchBadgerBlockStore(b))
		tree.store.resetCounts()
		b.ResetTimer()
		for i := range b.N {
			tx := newBenchReadTx(b, ctx, tree)
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
	})

	b.Run("updates_100/"+benchSizeName(size), func(b *testing.B) {
		ctx := context.Background()
		tree := buildBenchTreeWithStore(b, makeBenchKeys(size, benchKeySequential), newBenchBadgerBlockStore(b))
		tree.store.resetCounts()
		b.ResetTimer()
		for i := range b.N {
			btx, rootCursor := block.NewTransaction(tree.storeOps(), nil, tree.rootRef, nil)
			tx, err := NewTx(ctx, rootCursor, nil, true, nil)
			if err != nil {
				b.Fatal(err)
			}
			for updateIndex := range 100 {
				key := tree.keys[benchLookupIndex(i+updateIndex, size)]
				if err := tx.Set(ctx, key, benchValue(i+updateIndex+size)); err != nil {
					tx.Discard()
					b.Fatal(err)
				}
			}
			tx.Discard()
			if _, _, err := btx.Write(ctx, true); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		tree.store.reportMetrics(b, int64(b.N))
	})
}

// _ is a type assertion
var _ block.StoreOps = ((*benchBlockStore)(nil))

// _ is a type assertion
var _ block_gc.RefGraphOps = ((*benchRefGraph)(nil))
