//go:build js

package block_gc_test

import (
	"bytes"
	"context"
	"strconv"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/block/gc/gcgraph"
	block_gc_wal "github.com/s4wave/spacewave/db/block/gc/wal"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/opfs/filelock"
	"github.com/s4wave/spacewave/db/volume/js/opfs/blockshard"
	"github.com/s4wave/spacewave/net/hash"
)

// testHarness sets up real OPFS-backed block store, GC graph, and WAL writer.
type testHarness struct {
	t          *testing.T
	root       func() // cleanup
	blkStore   block.StoreOps
	engine     *blockshard.Engine
	gcGraph    *gcgraph.GCGraph
	walWriter  *block_gc_wal.Writer
	appender   *block_gc_wal.Appender
	lockPrefix string
}

func newTestHarness(t *testing.T, name string) *testHarness {
	t.Helper()
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}

	opfsRoot, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}

	dir, err := opfs.GetDirectory(opfsRoot, name, true)
	if err != nil {
		t.Fatal(err)
	}
	cleanup := func() { opfs.DeleteEntry(opfsRoot, name, true) } //nolint

	blocksDir, err := opfs.GetDirectory(dir, "blocks", true)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	gcDir, err := opfs.GetDirectory(dir, "gc", true)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	graphDir, err := opfs.GetDirectory(gcDir, "graph", true)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	walDir, err := opfs.GetDirectory(gcDir, "wal", true)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}

	lockPrefix := name
	ctx := context.Background()

	engine, err := blockshard.NewEngine(ctx, blocksDir, lockPrefix+"/blocks", blockshard.DefaultShardCount)
	if err != nil {
		cleanup()
		t.Fatal(err)
	}
	blkStore := blockshard.NewBlockStore(engine, hash.HashType_HashType_BLAKE3)

	gcGraph, err := gcgraph.NewGCGraph(graphDir, lockPrefix+"/gc/graph")
	if err != nil {
		cleanup()
		t.Fatal(err)
	}

	// Register volume-context roots.
	if err := gcGraph.AddRoot(ctx, block_gc.NodeGCRoot); err != nil {
		cleanup()
		t.Fatal(err)
	}
	if err := gcGraph.AddRoot(ctx, block_gc.NodeUnreferenced); err != nil {
		cleanup()
		t.Fatal(err)
	}

	stwLock := lockPrefix + "|gc-stw"
	orderLock := lockPrefix + "|gc-wal-order"
	walWriter := block_gc_wal.NewWriter(walDir, lockPrefix+"/gc/wal", orderLock, stwLock)
	appender := block_gc_wal.NewAppender(walWriter)

	return &testHarness{
		t:          t,
		root:       cleanup,
		blkStore:   blkStore,
		engine:     engine,
		gcGraph:    gcGraph,
		walWriter:  walWriter,
		appender:   appender,
		lockPrefix: lockPrefix,
	}
}

func (h *testHarness) cleanup() {
	if h.engine != nil {
		h.engine.Close()
	}
	h.root()
}

// newGCStoreOps creates a GCStoreOps wired to the harness block store,
// GC graph, and WAL appender, under the given parent IRI.
func (h *testHarness) newGCStoreOps(parentIRI string) *block_gc.GCStoreOps {
	ops := block_gc.NewGCStoreOpsWithParentAndTraceTask(
		h.blkStore,
		h.gcGraph,
		parentIRI,
		block_gc.BucketFlushTask(),
	)
	ops.SetWALAppender(h.appender)
	return ops
}

// replayWAL returns a WALReplayFunc that reads and applies WAL entries.
func (h *testHarness) replayWAL() block_gc.WALReplayFunc {
	return func(ctx context.Context, graph block_gc.CollectorGraph) (int, error) {
		entries, filenames, err := block_gc_wal.ReadWAL(h.walWriter.Dir(), h.lockPrefix+"/gc/wal")
		if err != nil {
			return 0, err
		}
		for i, entry := range entries {
			adds := make([]block_gc.RefEdge, len(entry.GetAdds()))
			for j, e := range entry.GetAdds() {
				adds[j] = block_gc.RefEdge{Subject: e.GetSubject(), Object: e.GetObject()}
			}
			removes := make([]block_gc.RefEdge, len(entry.GetRemoves()))
			for j, e := range entry.GetRemoves() {
				removes[j] = block_gc.RefEdge{Subject: e.GetSubject(), Object: e.GetObject()}
			}
			if err := graph.ApplyRefBatch(ctx, adds, removes); err != nil {
				return i, err
			}
			if err := block_gc_wal.DeleteWALEntry(h.walWriter.Dir(), filenames[i]); err != nil {
				return i, err
			}
		}
		return len(entries), nil
	}
}

// acquireSTW returns an STWLockFunc using the harness lock prefix.
func (h *testHarness) acquireSTW() block_gc.STWLockFunc {
	stwLock := h.lockPrefix + "|gc-stw"
	return func() (func(), error) {
		return filelock.AcquireWebLock(stwLock, true)
	}
}

// sweepTarget wraps the block store for GC sweep deletion.
type sweepTarget struct {
	blk block.StoreOps
}

func (s *sweepTarget) DeleteBlock(ctx context.Context, iri string) error {
	ref, ok := block_gc.ParseBlockIRI(iri)
	if !ok {
		return nil
	}
	return s.blk.RmBlock(ctx, ref)
}

func (s *sweepTarget) DeleteObject(_ context.Context, _ string) error {
	return nil
}

// TestGCIntegrationSweepUnreachable writes blocks through GCStoreOps with
// WAL, runs a sweep cycle, and verifies unreachable blocks are deleted
// while reachable blocks survive.
func TestGCIntegrationSweepUnreachable(t *testing.T) {
	h := newTestHarness(t, "test-gc-integ-sweep")
	defer h.cleanup()
	ctx := context.Background()

	bucketIRI := block_gc.BucketIRI("test-bucket")
	ops := h.newGCStoreOps(bucketIRI)

	// Write 3 blocks. Block 0 and 1 get a parent ref (bucket -> block).
	// Block 2 also gets a parent ref, but we'll remove it before sweep.
	var blockIRIs [3]string
	for i := range 3 {
		data := []byte("block-" + strconv.Itoa(i))
		ref, _, err := ops.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		blockIRIs[i] = block_gc.BlockIRI(ref)
	}
	if err := ops.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}

	// Add a block-to-block ref: block0 -> block1 (so block1 is reachable
	// even without the bucket parent if block0 is reachable).
	ref0, ok := block_gc.ParseBlockIRI(blockIRIs[0])
	if !ok {
		t.Fatal("bad block IRI 0")
	}
	ref1, ok := block_gc.ParseBlockIRI(blockIRIs[1])
	if !ok {
		t.Fatal("bad block IRI 1")
	}
	ops.bufferBlockRefs(ref0, []*block.BlockRef{ref1})
	if err := ops.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}

	// Remove block2 from the bucket (make it unreachable).
	ref2, ok := block_gc.ParseBlockIRI(blockIRIs[2])
	if !ok {
		t.Fatal("bad block IRI 2")
	}
	if err := ops.RmBlock(ctx, ref2); err != nil {
		t.Fatal(err)
	}

	// Run sweep.
	target := &sweepTarget{blk: h.blkStore}
	result, err := block_gc.SweepCycle(ctx, block_gc.SweepConfig{
		Graph:      h.gcGraph,
		Target:     target,
		ReplayWAL:  h.replayWAL(),
		AcquireSTW: h.acquireSTW(),
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Logf("sweep result: WAL1=%d WAL2=%d candidates=%d rescued=%d swept=%d",
		result.WALEntriesPhase1, result.WALEntriesPhase2,
		result.SweepCandidates, result.Rescued, result.Swept)

	// Block 0: reachable (bucket -> block0). Must survive.
	exists0, err := h.blkStore.GetBlockExists(ctx, ref0)
	if err != nil {
		t.Fatal(err)
	}
	if !exists0 {
		t.Error("block 0 was swept but should be reachable via bucket")
	}

	// Block 1: reachable (bucket -> block0 -> block1). Must survive.
	exists1, err := h.blkStore.GetBlockExists(ctx, ref1)
	if err != nil {
		t.Fatal(err)
	}
	if !exists1 {
		t.Error("block 1 was swept but should be reachable via block0")
	}

	// Block 2: unreachable (removed from bucket). Must be deleted.
	exists2, err := h.blkStore.GetBlockExists(ctx, ref2)
	if err != nil {
		t.Fatal(err)
	}
	if exists2 {
		t.Error("block 2 survived sweep but should be unreachable")
	}
}

// TestGCIntegrationConcurrentWriteAndSweep writes blocks from multiple
// goroutines while a sweep runs, verifying no data corruption.
func TestGCIntegrationConcurrentWriteAndSweep(t *testing.T) {
	h := newTestHarness(t, "test-gc-integ-conc")
	defer h.cleanup()
	ctx := context.Background()

	bucketIRI := block_gc.BucketIRI("conc-bucket")

	// Phase 1: Write some initial blocks that will be reachable.
	ops := h.newGCStoreOps(bucketIRI)
	for i := range 5 {
		data := []byte("initial-" + strconv.Itoa(i))
		if _, _, err := ops.PutBlock(ctx, data, nil); err != nil {
			t.Fatal(err)
		}
	}
	if err := ops.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}

	// Phase 2: Concurrent writers + sweep.
	var wg sync.WaitGroup
	const writers = 4
	const blocksPerWriter = 5

	// Collect all block IRIs so we can verify they survive.
	results := make([][]string, writers)

	for w := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wOps := h.newGCStoreOps(bucketIRI)
			var iris []string
			for j := range blocksPerWriter {
				data := []byte("writer-" + strconv.Itoa(w) + "-block-" + strconv.Itoa(j))
				ref, _, err := wOps.PutBlock(ctx, data, nil)
				if err != nil {
					t.Error(err)
					return
				}
				iris = append(iris, block_gc.BlockIRI(ref))
			}
			if err := wOps.FlushPending(ctx); err != nil {
				t.Error(err)
			}
			results[w] = iris
		}()
	}

	// Run a sweep concurrently with the writers.
	wg.Add(1)
	go func() {
		defer wg.Done()
		target := &sweepTarget{blk: h.blkStore}
		_, err := block_gc.SweepCycle(ctx, block_gc.SweepConfig{
			Graph:      h.gcGraph,
			Target:     target,
			ReplayWAL:  h.replayWAL(),
			AcquireSTW: h.acquireSTW(),
		})
		if err != nil {
			t.Error(err)
		}
	}()

	wg.Wait()
	if t.Failed() {
		return
	}

	// All written blocks should still exist (all are bucket-owned).
	for w, iris := range results {
		for j, iri := range iris {
			ref, ok := block_gc.ParseBlockIRI(iri)
			if !ok {
				t.Errorf("writer %d block %d: bad IRI %s", w, j, iri)
				continue
			}
			data, found, err := h.blkStore.GetBlock(ctx, ref)
			if err != nil {
				t.Errorf("writer %d block %d: %v", w, j, err)
				continue
			}
			if !found {
				t.Errorf("writer %d block %d: not found after sweep (IRI %s)", w, j, iri)
				continue
			}
			want := []byte("writer-" + strconv.Itoa(w) + "-block-" + strconv.Itoa(j))
			if !bytes.Equal(data, want) {
				t.Errorf("writer %d block %d: got %q, want %q", w, j, data, want)
			}
		}
	}
}
