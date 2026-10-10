package block_gc

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
)

type failingTraversal struct{ CollectorGraph }

func (g failingTraversal) GetOutgoingRefs(context.Context, string) ([]string, error) {
	return nil, errInjectedRefBatch
}

func TestMarkerRefusesIncompleteReachability(t *testing.T) {
	// Connect the live node to its root before injecting traversal failure.
	graph := newMockGraph()
	graph.addRoot("root")
	graph.addEdge("root", "live")

	// Verify a failed traversal cannot authorize sweep candidates.
	candidates, _, err := NewMarker(failingTraversal{graph}).Mark(t.Context())
	if !errors.Is(err, errInjectedRefBatch) || len(candidates) != 0 {
		t.Fatalf("failed mark authorized candidates %v: %v", candidates, err)
	}
}

type retryAppender struct {
	calls int
	adds  []RefEdge
}

func (w *retryAppender) Append(_ context.Context, adds, _ []RefEdge) error {
	w.calls++
	if w.calls == 1 {
		return errInjectedRefBatch
	}
	w.adds = append(w.adds, adds...)
	return nil
}

// GetPendingOutgoingRefs reports no journaled edges.
func (*retryAppender) GetPendingOutgoingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestFlushRetainsFailedWALAppend(t *testing.T) {
	// Create a GC store with a WAL append that fails once.
	wal := &retryAppender{}
	store := NewGCStoreOps(block.NopStoreOps{}, nil)
	store.SetWALAppender(wal)
	store.pendingUnref = []string{"block:test"}

	// Verify a failed WAL append is reported.
	if err := store.FlushPending(t.Context()); !errors.Is(err, errInjectedRefBatch) {
		t.Fatal(err)
	}

	// Flush the retained WAL or staging changes.
	if err := store.FlushPending(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Verify the repeated flush retains the failed WAL batch.
	if len(wal.adds) != 1 || wal.adds[0].Object != "block:test" {
		t.Fatalf("lost WAL batch: %v", wal.adds)
	}
}

func TestReleasePreservesSharedBlockDependencies(t *testing.T) {
	for _, batch := range []bool{false, true} {
		// Choose the shared block release path.
		name := "single"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			// Store a shared parent-child graph with two owners.
			env := newGCTestEnvWithParent(t, "owner-a")
			parent := env.putBlock(t, "shared-parent")
			child := env.putBlock(t, "shared-child")
			env.recordRefs(parent, []*block.BlockRef{child})

			// Flush pending ownership changes into the graph.
			env.flush(t)
			if err := env.refGraph.AddRef(env.ctx, "owner-b", BlockIRI(parent)); err != nil {
				t.Fatal(err)
			}

			// Release the first owner through the selected path.
			var err error
			if batch {
				_, err = env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{{Ref: parent, Tombstone: true}})
			} else {
				err = env.gcStore.RmBlock(env.ctx, parent)
			}
			if err != nil {
				t.Fatal(err)
			}

			// Flush pending ownership changes into the graph.
			env.flush(t)

			// Collect orphan nodes after the ownership change.
			collector := NewCollector(env.refGraph, env.rawStore, nil)
			if _, err := collector.Collect(env.ctx); err != nil {
				t.Fatal(err)
			}

			// Verify the surviving owner retains the shared child edge.
			outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, BlockIRI(parent))
			if err != nil || len(outgoing) != 1 || outgoing[0] != BlockIRI(child) {
				t.Fatalf("release lost shared dependencies: %v %v", outgoing, err)
			}

			// Remove the last owner of the shared parent block.
			if err := env.gcStore.RemoveGCRef(env.ctx, "owner-b", BlockIRI(parent)); err != nil {
				t.Fatal(err)
			}

			// Verify collection sweeps the graph after its last owner leaves.
			stats, err := collector.Collect(env.ctx)
			if err != nil || stats.NodesSwept != 2 {
				t.Fatalf("last release did not collect parent and child: %+v %v", stats, err)
			}
		})
	}
}

type blockingAppender struct{ entered, release chan struct{} }

func (w blockingAppender) Append(context.Context, []RefEdge, []RefEdge) error {
	close(w.entered)
	<-w.release
	return nil
}

// GetPendingOutgoingRefs reports no journaled edges.
func (blockingAppender) GetPendingOutgoingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func TestConcurrentFlushCanCancelWhileOlderDeliveryRuns(t *testing.T) {
	// Start a GC flush whose WAL delivery waits for release.
	wal := blockingAppender{make(chan struct{}), make(chan struct{})}
	store := NewGCStoreOps(block.NopStoreOps{}, nil)
	store.SetWALAppender(wal)
	store.pendingUnref = []string{"block:test"}
	finished := make(chan error, 1)
	go func() { finished <- store.FlushPending(t.Context()) }()
	<-wal.entered

	// Cancel the context for the overlapping flush.
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	// Release the older delivery after the overlapping flush returns.
	err := store.FlushPending(ctx)
	close(wal.release)

	// Verify the overlapping flush returns cancellation.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting flush returned %v", err)
	}

	// Verify the older flush completes after delivery is released.
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestSweepRechecksReachabilityAfterAnotherReplayDrainsWAL(t *testing.T) {
	// Create an initially unreachable candidate beside the graph root.
	graph := newMockGraph()
	graph.addRoot("root")
	graph.addNode("candidate")

	// Run the sweep while another replay restores candidate reachability.
	result, err := SweepCycle(t.Context(), SweepConfig{
		Graph: graph, Target: &mockSweepTarget{}, ReplayWAL: noopReplay,
		AcquireSTW: func() (func(), error) {
			graph.addEdge("root", "candidate")
			return func() {}, nil
		},
	})

	// Verify the restored candidate is rescued from deletion.
	if err != nil || result.Rescued != 1 || result.Swept != 0 {
		t.Fatalf("lost rescue after external replay: %+v %v", result, err)
	}
}

func TestBatchPreservesLastOwnershipOperation(t *testing.T) {
	for _, releaseLast := range []bool{false, true} {
		// Choose the order of the put and release entries.
		name := "put-last"
		if releaseLast {
			name = "release-last"
		}
		t.Run(name, func(t *testing.T) {
			// Store the owned block for the ordering check.
			env := newGCTestEnvWithParent(t, "owner")
			ref := env.putBlock(t, "batch-order")

			// Flush pending ownership changes into the graph.
			env.flush(t)

			// Read the block payload for the repeated put entry.
			data, _, err := env.rawStore.GetBlock(env.ctx, ref)
			if err != nil {
				t.Fatal(err)
			}

			// Order put and release entries for the same block.
			put := &block.PutBatchEntry{Ref: ref, Data: data}
			release := &block.PutBatchEntry{Ref: ref, Tombstone: true}
			entries := []*block.PutBatchEntry{release, put}
			if releaseLast {
				entries = []*block.PutBatchEntry{put, release}
			}

			// Write the ordered ownership operations as one batch.
			if _, err := env.gcStore.PutBlockBatch(env.ctx, entries); err != nil {
				t.Fatal(err)
			}

			// Flush pending ownership changes into the graph.
			env.flush(t)

			// Verify ownership follows the last batch operation.
			owned, err := env.refGraph.HasIncomingRefs(env.ctx, BlockIRI(ref))
			if err != nil || owned == releaseLast {
				t.Fatalf("last operation not respected: owned=%v, err=%v", owned, err)
			}
		})
	}
}

func TestCollectorRetriesFailedCleanup(t *testing.T) {
	// Store the orphan block for the recovery check.
	env := newGCTestEnv(t)
	ref := env.putBlock(t, "retry-cleanup")

	// Flush pending ownership changes into the graph.
	env.flush(t)

	// Create a collector whose cleanup callback fails once.
	calls := 0
	collector := NewCollector(env.refGraph, env.rawStore, func(context.Context, string) error {
		// Fail the first cleanup attempt and allow its next attempt.
		calls++
		if calls == 1 {
			return errInjectedRefBatch
		}
		return nil
	})

	// Verify the first collection reports its cleanup failure.
	if _, err := collector.Collect(env.ctx); !errors.Is(err, errInjectedRefBatch) {
		t.Fatal(err)
	}

	// Verify repeated collection completes the failed orphan cleanup.
	stats, err := collector.Collect(env.ctx)
	if err != nil || stats.NodesSwept != 1 {
		t.Fatalf("lost cleanup retry: %+v %v", stats, err)
	}

	// Verify the successful cleanup retry removes the physical block.
	if exists, err := env.rawStore.GetBlockExists(env.ctx, ref); err != nil || exists {
		t.Fatalf("block survived retry: exists=%v err=%v", exists, err)
	}
}

type rejectingSweepStore struct{ block.StoreOps }

func (rejectingSweepStore) SweepUnreferenced(context.Context, RefGraphOps, []string) ([]string, error) {
	return nil, ErrAtomicSweepUnsupported
}

func TestCollectorDoesNotBypassAtomicScopeRejection(t *testing.T) {
	// Store the orphan block for the recovery check.
	env := newGCTestEnv(t)
	ref := env.putBlock(t, "wrong-scope")

	// Create a collector whose block store rejects atomic sweeping.
	collector := NewCollector(env.refGraph, rejectingSweepStore{env.rawStore}, nil)

	// Verify the collector reports an unsupported atomic scope.
	if _, err := collector.Collect(env.ctx); !errors.Is(err, ErrAtomicSweepUnsupported) {
		t.Fatal(err)
	}

	// Verify rejecting the atomic scope preserves the physical block.
	if exists, err := env.rawStore.GetBlockExists(env.ctx, ref); err != nil || !exists {
		t.Fatalf("deleted outside atomic scope: exists=%v err=%v", exists, err)
	}
}

func TestDeferFlushRecoversFromUnmatchedEnd(t *testing.T) {
	// Create a GC store for deferred-flush scope recovery.
	store := NewGCStoreOps(block.NopStoreOps{}, &recordingRefGraph{})

	// Verify an unmatched deferred-flush end is rejected.
	if err := store.EndDeferFlush(t.Context()); err == nil {
		t.Fatal("accepted unmatched end")
	}

	// Buffer a staging edge inside a new deferred-flush scope.
	store.BeginDeferFlush()
	store.pendingUnref = []string{"block:test"}

	// Flush the retained WAL or staging changes.
	if err := store.FlushPending(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Verify the new deferred scope retains its pending staging edge.
	if len(store.pendingUnref) != 1 {
		t.Fatal("unmatched end poisoned the next deferred scope")
	}

	// Close the valid deferred-flush scope.
	if err := store.EndDeferFlush(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Verify closing the scope flushes its staging edge.
	if len(store.pendingUnref) != 0 {
		t.Fatal("closing the scope did not flush")
	}
}

func TestPutThenReleaseRetainsExistingStagingMark(t *testing.T) {
	// Store the orphan block for the recovery check.
	env := newGCTestEnv(t)
	ref := env.putBlock(t, "staged-release")

	// Read the block payload for the repeated put entry.
	data, _, err := env.rawStore.GetBlock(env.ctx, ref)
	if err != nil {
		t.Fatal(err)
	}

	// Put and release the already-staged block through a parent store.
	owner := NewGCStoreOpsWithParent(env.rawStore, env.refGraph, "owner")
	if _, err := owner.PutBlockBatch(env.ctx, []*block.PutBatchEntry{
		{Ref: ref, Data: data}, {Ref: ref, Tombstone: true},
	}); err != nil {
		t.Fatal(err)
	}

	// Flush the parent ownership operations.
	if err := owner.FlushPending(env.ctx); err != nil {
		t.Fatal(err)
	}

	// Verify collection still sees the original staging mark.
	stats, err := NewCollector(env.refGraph, env.rawStore, nil).Collect(env.ctx)
	if err != nil || stats.NodesSwept != 1 {
		t.Fatalf("lost staging marker: %+v %v", stats, err)
	}
}
