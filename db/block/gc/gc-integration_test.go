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
	volume_browser "github.com/s4wave/spacewave/db/volume/browser"
	"github.com/sirupsen/logrus"
)

// testHarness exposes one browser volume's block, graph, and journal hooks.
type testHarness struct {
	// t reports cleanup failures.
	t *testing.T
	// volume owns the disposable persisted volume.
	volume *volume_browser.Volume
	// blkStore provides the public block operations.
	blkStore block.StoreOps
	// gcGraph provides the public collector graph.
	gcGraph block_gc.CollectorGraph
	// appender records durable reference changes.
	appender block_gc.WALAppender
	// hooks provides replay and stop-the-world boundaries.
	hooks block_gc.ManagerHooks
}

// newTestHarness opens the product volume and requires its GC integration hooks.
func newTestHarness(t *testing.T, name string) *testHarness {
	// Open the product volume that owns the block store and GC state.
	t.Helper()
	ctx := context.Background()
	volume, err := volume_browser.NewVolume(
		ctx,
		logrus.NewEntry(logrus.New()),
		&volume_browser.Config{Name: name},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require the graph, journal, and sweep hooks exposed by the volume.
	hooks, ok := volume.GetGCManagerHooks()
	if !ok {
		if err := volume.Delete(); err != nil {
			t.Errorf("delete test volume: %v", err)
		}
		t.Fatal("browser volume has no GC manager hooks")
	}
	appender := volume.GetWALAppender()
	if appender == nil {
		if err := volume.Delete(); err != nil {
			t.Errorf("delete test volume: %v", err)
		}
		t.Fatal("browser volume has no GC journal appender")
	}

	// Retain only public interfaces consumed by the observable sweep tests.
	return &testHarness{
		t:        t,
		volume:   volume,
		blkStore: volume,
		gcGraph:  hooks.Graph,
		appender: appender,
		hooks:    hooks,
	}
}

// cleanup deletes only this test's disposable volume.
func (h *testHarness) cleanup() {
	if err := h.volume.Delete(); err != nil {
		h.t.Errorf("delete test volume: %v", err)
	}
}

// newGCStoreOps creates a GCStoreOps wired to the harness block store,
// GC graph, and journal appender, under the given parent IRI. It roots the
// parent as a bucket handle does, so the marker reaches the parent's blocks.
func (h *testHarness) newGCStoreOps(parentIRI string) *block_gc.GCStoreOps {
	// Root the parent node.
	if err := h.gcGraph.AddRef(context.Background(), block_gc.NodeGCRoot, parentIRI); err != nil {
		h.t.Errorf("root %s: %v", parentIRI, err)
	}

	// Track the parent's block writes in the graph and journal.
	ops := block_gc.NewGCStoreOpsWithParentAndTraceTask(
		h.blkStore,
		h.gcGraph,
		parentIRI,
		block_gc.BucketFlushTask(),
	)
	ops.SetWALAppender(h.appender)
	return ops
}

// sweepTarget wraps the block store for GC sweep deletion.
type sweepTarget struct {
	// blk provides block deletion during sweep.
	blk block.StoreOps
}

// DeleteBlock removes a parsed block reference through the public store.
func (s *sweepTarget) DeleteBlock(ctx context.Context, iri string) error {
	ref, ok := block_gc.ParseBlockIRI(iri)
	if !ok {
		return nil
	}
	return s.blk.RmBlock(ctx, ref)
}

// DeleteObject accepts object deletion in these block-only sweep fixtures.
func (s *sweepTarget) DeleteObject(_ context.Context, _ string) error {
	return nil
}

// TestGCIntegrationSweepUnreachable writes blocks through journaled GCStoreOps,
// runs a sweep cycle, and verifies unreachable blocks are deleted
// while reachable blocks survive.
func TestGCIntegrationSweepUnreachable(t *testing.T) {
	// Open a harness with a rooted bucket.
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
	if _, _, err := ops.PutBlock(ctx, []byte("block-0"), &block.PutOpts{
		Refs: []*block.BlockRef{ref1},
	}); err != nil {
		t.Fatal(err)
	}
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
	if err := ops.FlushPending(ctx); err != nil {
		t.Fatal(err)
	}

	// Run sweep.
	target := &sweepTarget{blk: h.blkStore}
	result, err := block_gc.SweepCycle(ctx, block_gc.SweepConfig{
		Graph:      h.gcGraph,
		Target:     target,
		ReplayWAL:  h.hooks.ReplayWAL,
		AcquireSTW: h.hooks.AcquireSTW,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Log the sweep counts.
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
	// Open the volume and its bucket.
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

	// Each writer stores its blocks and collects their IRIs so we can verify
	// they survive.
	results := make([][]string, writers)
	for w := range writers {
		wg.Add(1)
		go func() {
			// Put and flush the writer's blocks.
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
			ReplayWAL:  h.hooks.ReplayWAL,
			AcquireSTW: h.hooks.AcquireSTW,
		})
		if err != nil {
			t.Error(err)
		}
	}()

	// Wait for the writers and the sweep.
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
