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
	graph := newMockGraph()
	graph.addRoot("root")
	graph.addEdge("root", "live")
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
	wal := &retryAppender{}
	store := NewGCStoreOps(block.NopStoreOps{}, nil)
	store.SetWALAppender(wal)
	store.pendingUnref = []string{"block:test"}
	if err := store.FlushPending(t.Context()); !errors.Is(err, errInjectedRefBatch) {
		t.Fatal(err)
	}
	if err := store.FlushPending(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(wal.adds) != 1 || wal.adds[0].Object != "block:test" {
		t.Fatalf("lost WAL batch: %v", wal.adds)
	}
}

func TestReleasePreservesSharedBlockDependencies(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "single"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			env := newGCTestEnvWithParent(t, "owner-a")
			parent := env.putBlock(t, "shared-parent")
			child := env.putBlock(t, "shared-child")
			env.recordRefs(parent, []*block.BlockRef{child})
			env.flush(t)
			if err := env.refGraph.AddRef(env.ctx, "owner-b", BlockIRI(parent)); err != nil {
				t.Fatal(err)
			}
			var err error
			if batch {
				err = env.gcStore.PutBlockBatch(env.ctx, []*block.PutBatchEntry{{Ref: parent, Tombstone: true}})
			} else {
				err = env.gcStore.RmBlock(env.ctx, parent)
			}
			if err != nil {
				t.Fatal(err)
			}
			env.flush(t)
			collector := NewCollector(env.refGraph, env.rawStore, nil)
			if _, err := collector.Collect(env.ctx); err != nil {
				t.Fatal(err)
			}
			outgoing, err := env.refGraph.GetOutgoingRefs(env.ctx, BlockIRI(parent))
			if err != nil || len(outgoing) != 1 || outgoing[0] != BlockIRI(child) {
				t.Fatalf("release lost shared dependencies: %v %v", outgoing, err)
			}
			if err := env.gcStore.RemoveGCRef(env.ctx, "owner-b", BlockIRI(parent)); err != nil {
				t.Fatal(err)
			}
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
	wal := blockingAppender{make(chan struct{}), make(chan struct{})}
	store := NewGCStoreOps(block.NopStoreOps{}, nil)
	store.SetWALAppender(wal)
	store.pendingUnref = []string{"block:test"}
	finished := make(chan error, 1)
	go func() { finished <- store.FlushPending(t.Context()) }()
	<-wal.entered
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := store.FlushPending(ctx)
	close(wal.release)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting flush returned %v", err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestSweepRechecksReachabilityAfterAnotherReplayDrainsWAL(t *testing.T) {
	graph := newMockGraph()
	graph.addRoot("root")
	graph.addNode("candidate")
	result, err := SweepCycle(t.Context(), SweepConfig{
		Graph: graph, Target: &mockSweepTarget{}, ReplayWAL: noopReplay,
		AcquireSTW: func() (func(), error) {
			graph.addEdge("root", "candidate")
			return func() {}, nil
		},
	})
	if err != nil || result.Rescued != 1 || result.Swept != 0 {
		t.Fatalf("lost rescue after external replay: %+v %v", result, err)
	}
}

func TestBatchPreservesLastOwnershipOperation(t *testing.T) {
	for _, releaseLast := range []bool{false, true} {
		name := "put-last"
		if releaseLast {
			name = "release-last"
		}
		t.Run(name, func(t *testing.T) {
			env := newGCTestEnvWithParent(t, "owner")
			ref := env.putBlock(t, "batch-order")
			env.flush(t)
			data, _, err := env.rawStore.GetBlock(env.ctx, ref)
			if err != nil {
				t.Fatal(err)
			}
			put := &block.PutBatchEntry{Ref: ref, Data: data}
			release := &block.PutBatchEntry{Ref: ref, Tombstone: true}
			entries := []*block.PutBatchEntry{release, put}
			if releaseLast {
				entries = []*block.PutBatchEntry{put, release}
			}
			if err := env.gcStore.PutBlockBatch(env.ctx, entries); err != nil {
				t.Fatal(err)
			}
			env.flush(t)
			owned, err := env.refGraph.HasIncomingRefs(env.ctx, BlockIRI(ref))
			if err != nil || owned == releaseLast {
				t.Fatalf("last operation not respected: owned=%v, err=%v", owned, err)
			}
		})
	}
}

func TestCollectorRetriesFailedCleanup(t *testing.T) {
	env := newGCTestEnv(t)
	ref := env.putBlock(t, "retry-cleanup")
	env.flush(t)
	calls := 0
	collector := NewCollector(env.refGraph, env.rawStore, func(context.Context, string) error {
		calls++
		if calls == 1 {
			return errInjectedRefBatch
		}
		return nil
	})
	if _, err := collector.Collect(env.ctx); !errors.Is(err, errInjectedRefBatch) {
		t.Fatal(err)
	}
	stats, err := collector.Collect(env.ctx)
	if err != nil || stats.NodesSwept != 1 {
		t.Fatalf("lost cleanup retry: %+v %v", stats, err)
	}
	if exists, err := env.rawStore.GetBlockExists(env.ctx, ref); err != nil || exists {
		t.Fatalf("block survived retry: exists=%v err=%v", exists, err)
	}
}

type rejectingSweepStore struct{ block.StoreOps }

func (rejectingSweepStore) SweepUnreferenced(context.Context, RefGraphOps, []string) ([]string, error) {
	return nil, ErrAtomicSweepUnsupported
}

func TestCollectorDoesNotBypassAtomicScopeRejection(t *testing.T) {
	env := newGCTestEnv(t)
	ref := env.putBlock(t, "wrong-scope")
	collector := NewCollector(env.refGraph, rejectingSweepStore{env.rawStore}, nil)
	if _, err := collector.Collect(env.ctx); !errors.Is(err, ErrAtomicSweepUnsupported) {
		t.Fatal(err)
	}
	if exists, err := env.rawStore.GetBlockExists(env.ctx, ref); err != nil || !exists {
		t.Fatalf("deleted outside atomic scope: exists=%v err=%v", exists, err)
	}
}

func TestDeferFlushRecoversFromUnmatchedEnd(t *testing.T) {
	store := NewGCStoreOps(block.NopStoreOps{}, &recordingRefGraph{})
	if err := store.EndDeferFlush(t.Context()); err == nil {
		t.Fatal("accepted unmatched end")
	}
	store.BeginDeferFlush()
	store.pendingUnref = []string{"block:test"}
	if err := store.FlushPending(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(store.pendingUnref) != 1 {
		t.Fatal("unmatched end poisoned the next deferred scope")
	}
	if err := store.EndDeferFlush(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(store.pendingUnref) != 0 {
		t.Fatal("closing the scope did not flush")
	}
}

func TestPutThenReleaseRetainsExistingStagingMark(t *testing.T) {
	env := newGCTestEnv(t)
	ref := env.putBlock(t, "staged-release")
	data, _, err := env.rawStore.GetBlock(env.ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	owner := NewGCStoreOpsWithParent(env.rawStore, env.refGraph, "owner")
	if err := owner.PutBlockBatch(env.ctx, []*block.PutBatchEntry{
		{Ref: ref, Data: data}, {Ref: ref, Tombstone: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := owner.FlushPending(env.ctx); err != nil {
		t.Fatal(err)
	}
	stats, err := NewCollector(env.refGraph, env.rawStore, nil).Collect(env.ctx)
	if err != nil || stats.NodesSwept != 1 {
		t.Fatalf("lost staging marker: %+v %v", stats, err)
	}
}
