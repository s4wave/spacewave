package kvtx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

func orphanPublication(t *testing.T, v *Volume, p *block.AtomicPublication) string {
	// Remove the publication bucket edge to orphan its block.
	t.Helper()
	node := block_gc.BlockIRI(p.Entries[0].Ref)
	if err := v.GetRefGraph().ApplyRefBatch(t.Context(), nil, []block_gc.RefEdge{{Subject: block_gc.BucketIRI(p.BucketID), Object: node}}); err != nil {
		t.Fatal(err)
	}

	// Verify the reference graph marks the publication block as a sweep candidate.
	candidates, err := v.GetRefGraph().GetUnreferencedNodes(t.Context())
	if err != nil || !slices.Contains(candidates, node) {
		t.Fatalf("candidate %q missing: %v %v", node, candidates, err)
	}
	return node
}

func TestPublicationSweepRechecksRescuedCandidate(t *testing.T) {
	// Publish a block and release its bucket ownership.
	v, _ := newPublicationTestVolume(t)
	p := publicationFor(t, "sweep", "", "one")
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	node := orphanPublication(t, v, p)

	// The collector already holds node in a candidate snapshot. Publication
	// rescues the same bytes before the physical deletion can begin.
	p.Heads = publicationFor(t, "sweep", "one", "two").Heads
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}

	// Verify atomic sweeping preserves the rescued block and its new head.
	removed, err := v.SweepUnreferenced(t.Context(), v.GetRefGraph(), []string{node})
	if err != nil || len(removed) != 0 {
		t.Fatalf("rescued candidate: removed=%v err=%v", removed, err)
	}
	assertPublishedBlock(t, v, p, true)
	assertHead(t, v, "sweep", "two")
}

// TestPublicationSweepBatchMarksReleasedChildren checks that one sweep batch
// marks each child it leaves without an owner, leaves a child another owner
// keeps unmarked, and leaves no marker on the nodes it removes.
func TestPublicationSweepBatchMarksReleasedChildren(t *testing.T) {
	// Mark two orphans: one shares a child with a kept node, both own another.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	rg := v.GetRefGraph()
	unref := block_gc.NodeUnreferenced
	err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
		{Subject: block_gc.NodeGCRoot, Object: "keep"},
		{Subject: "keep", Object: "shared"},
		{Subject: unref, Object: "o1"},
		{Subject: unref, Object: "o2"},
		{Subject: "o1", Object: "child"},
		{Subject: "o1", Object: "shared"},
		{Subject: "o2", Object: "child"},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Sweep both in one batch, with a duplicate candidate.
	removed, err := v.SweepUnreferenced(ctx, rg, []string{"o1", "o2", "o1"})
	if err != nil || !slices.Equal(removed, []string{"o1", "o2"}) {
		t.Fatalf("sweep: removed=%v err=%v", removed, err)
	}

	// Only the released child gains a marker.
	for node, want := range map[string][]string{
		"o1":     nil,
		"o2":     nil,
		"child":  {unref},
		"shared": {"keep"},
	} {
		got, err := rg.GetIncomingRefs(ctx, node)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, want) {
			t.Errorf("%s owners = %v, want %v", node, got, want)
		}
	}
}

type rescueSweepStore struct {
	*Volume
	rescue func() error
}

func (v *rescueSweepStore) SweepUnreferenced(ctx context.Context, graph block_gc.RefGraphOps, nodes []string) ([]string, error) {
	if v.rescue != nil {
		rescue := v.rescue
		v.rescue = nil
		if err := rescue(); err != nil {
			return nil, err
		}
	}
	return v.Volume.SweepUnreferenced(ctx, graph, nodes)
}

func TestPublicationCollectorUsesAtomicSweep(t *testing.T) {
	// Publish and orphan the collector test block.
	v, _ := newPublicationTestVolume(t)
	p := publicationFor(t, "collector", "", "one")
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, p)

	// Rescue the block during collection and verify atomic sweeping retains it.
	store := &rescueSweepStore{Volume: v, rescue: func() error {
		_, err := v.PrepareOwnedBlockBatch(t.Context(), p.BucketID, p.Entries)
		return err
	}}
	stats, err := block_gc.NewCollector(v.GetRefGraph(), store, nil).Collect(t.Context())
	if err != nil || stats.NodesSwept != 0 || stats.AtomicSweepCount == 0 {
		t.Fatalf("collector stats=%+v err=%v", stats, err)
	}
	assertPublishedBlock(t, v, p, true)

	// Release the rescued block and verify the collector deletes it.
	orphanPublication(t, v, p)
	stats, err = block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(t.Context())
	if err != nil || stats.NodesSwept != 1 || stats.RemoveBlockCount != 1 {
		t.Fatalf("orphan stats=%+v err=%v", stats, err)
	}
	assertPublishedBlock(t, v, p, false)

	// Sweep is idempotent after the marker and block have gone.
	removed, err := v.SweepUnreferenced(t.Context(), v.GetRefGraph(), []string{block_gc.BlockIRI(p.Entries[0].Ref)})
	if len(removed) != 0 || err != nil {
		t.Fatalf("repeat sweep: %v %v", removed, err)
	}
}

func TestPublicationCollectorSweepsCandidatesInOneCommit(t *testing.T) {
	// Publish and orphan a batch of collector candidates.
	v, raw := newPublicationTestVolume(t)
	var published []*block.AtomicPublication
	for i := range 5 {
		p := publicationFor(t, fmt.Sprintf("batch-%d", i), "", "one")
		if err := v.PublishAtomic(t.Context(), p); err != nil {
			t.Fatal(err)
		}
		orphanPublication(t, v, p)
		published = append(published, p)
	}

	// Record the physical commit count before collecting the candidates.
	raw.mu.Lock()
	before := raw.commits
	raw.mu.Unlock()

	// Collect the orphaned batch and verify every block is swept.
	stats, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(t.Context())
	if err != nil || stats.NodesSwept != len(published) || stats.RemoveBlockCount != len(published) {
		t.Fatalf("collector stats=%+v err=%v", stats, err)
	}

	// Verify the collector sweeps the whole batch in one physical commit.
	raw.mu.Lock()
	count := raw.commits - before
	raw.mu.Unlock()
	if count != 1 {
		t.Fatalf("sweep used %d physical commits for %d candidates", count, len(published))
	}
	for _, p := range published {
		assertPublishedBlock(t, v, p, false)
	}
}

func TestPublicationSweepFailureRollsBackGraphAndBlock(t *testing.T) {
	// Publish and orphan a block for the failed sweep.
	v, raw := newPublicationTestVolume(t)
	p := publicationFor(t, "sweep-failure", "", "one")
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	node := orphanPublication(t, v, p)

	// Inject a physical commit failure into the atomic sweep.
	failure := errors.New("sync failed")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()

	// Verify the failed sweep reports no deleted nodes.
	removed, err := v.SweepUnreferenced(t.Context(), v.GetRefGraph(), []string{node})
	if len(removed) != 0 || !errors.Is(err, failure) {
		t.Fatalf("failed sweep: %v %v", removed, err)
	}

	// Clear the commit fault before inspecting stored state.
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()

	// Verify the failed sweep preserves block bytes and the orphan marker.
	data, found, err := v.GetBlock(t.Context(), p.Entries[0].Ref)
	if err != nil || !found || !bytes.Equal(data, p.Entries[0].Data) {
		t.Fatal("failed sweep deleted bytes", err)
	}
	incoming, err := v.GetRefGraph().GetIncomingRefs(t.Context(), node)
	if err != nil || !slices.Contains(incoming, block_gc.NodeUnreferenced) {
		t.Fatal("failed sweep deleted marker", incoming, err)
	}

	// Verify a later sweep removes the preserved orphan.
	removed, err = v.SweepUnreferenced(t.Context(), v.GetRefGraph(), []string{node})
	if len(removed) != 1 || err != nil {
		t.Fatalf("retry sweep: %v %v", removed, err)
	}
}

func TestPublicationPreparationAtomicAndOversized(t *testing.T) {
	// Open a Volume for direct oversized block preparation.
	v, raw := newPublicationTestVolume(t)

	// Larger than the World staging cap: direct preparation is not admitted
	// into the bounded publication queue and cannot publish a head.
	data := bytes.Repeat([]byte("large-body"), (5<<20)/10)
	raw.mu.Lock()
	before := raw.commits
	raw.mu.Unlock()

	// Prepare the oversized block outside the publication queue.
	ref, existed, err := v.PrepareOwnedBlock(t.Context(), "large", data, nil)
	if err != nil || existed {
		t.Fatalf("prepare: %v %v", existed, err)
	}

	// Verify direct preparation uses one physical commit.
	raw.mu.Lock()
	count := raw.commits - before
	raw.mu.Unlock()
	if count != 1 {
		t.Fatalf("preparation used %d physical commits", count)
	}

	// Verify preparation stores owned bytes without publishing a head.
	p := &block.AtomicPublication{BucketID: "large", Entries: []*block.PutBatchEntry{{Ref: ref, Data: data}}}
	assertPublishedBlock(t, v, p, true)
	assertHead(t, v, "large", "")
	if stats := v.GetPublicationStats(); stats.Accepted != 0 {
		t.Fatalf("preparation was queued: %+v", stats)
	}

	// Verify preparing the same bytes reports the existing block.
	_, existed, err = v.PrepareOwnedBlock(t.Context(), "large", data, nil)
	if err != nil || !existed {
		t.Fatalf("duplicate preparation: %v %v", existed, err)
	}

	// Inject a physical commit failure into block batch preparation.
	failure := errors.New("preparation commit failed")
	failed := publicationFor(t, "unpublished", "", "one")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()

	// Attempt the failing batch and clear the injected fault.
	_, err = v.PrepareOwnedBlockBatch(t.Context(), failed.BucketID, failed.Entries)
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()

	// Verify failed preparation stores no owned block.
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	assertPublishedBlock(t, v, failed, false)
}

func TestPublicationDirectCloseAndGraphScope(t *testing.T) {
	// Verify atomic sweeping rejects another Volume reference graph.
	v, _ := newPublicationTestVolume(t)
	other, _ := newPublicationTestVolume(t)
	removed, err := v.SweepUnreferenced(t.Context(), other.GetRefGraph(), []string{"object:wrong-scope"})
	if len(removed) != 0 || !errors.Is(err, block_gc.ErrAtomicSweepUnsupported) {
		t.Fatalf("wrong scope: %v %v", removed, err)
	}

	// Close the Volume and verify it rejects direct preparation.
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = v.PrepareOwnedBlock(t.Context(), "closed", []byte("data"), nil)
	if !errors.Is(err, block.ErrPublicationClosed) {
		t.Fatal("preparation admitted after close", err)
	}
}

func TestPublicationGraphFailureRetainsWholeTransition(t *testing.T) {
	// Build a reference batch spanning multiple virtual graph slices.
	v, raw := newPublicationTestVolume(t)
	adds := make([]block_gc.RefEdge, 4100) // crosses RefGraph's virtual slice boundary
	for i := range adds {
		adds[i] = block_gc.RefEdge{Subject: "retry-owner", Object: fmt.Sprintf("object:retry-%d", i)}
	}

	// Inject a physical commit failure into the reference batch.
	failure := errors.New("outer graph commit failed")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()

	// Attempt the reference transition and clear the commit fault.
	err := v.GetRefGraph().ApplyRefBatch(t.Context(), adds, nil)
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()

	// Verify the failure retains the whole reference transition for retry.
	ra, rr, ok := block_gc.RefBatchRemainder(err)
	if !errors.Is(err, failure) || !ok || len(ra) != len(adds) || len(rr) != 0 {
		t.Fatalf("lost retry prefix: %v adds=%d removes=%d", err, len(ra), len(rr))
	}

	// Verify the failed physical transaction leaves no reference edges.
	got, err := v.GetRefGraph().GetOutgoingRefs(t.Context(), "retry-owner")
	if err != nil || len(got) != 0 {
		t.Fatalf("failed graph leaked edges: %d %v", len(got), err)
	}

	// Retry the retained transition and verify every edge is stored.
	if err := v.GetRefGraph().ApplyRefBatch(t.Context(), ra, rr); err != nil {
		t.Fatal(err)
	}
	got, err = v.GetRefGraph().GetOutgoingRefs(t.Context(), "retry-owner")
	if err != nil || len(got) != len(adds) {
		t.Fatalf("retry edges=%d want=%d: %v", len(got), len(adds), err)
	}
}

func TestPublicationCloseJoinsDirectPreparation(t *testing.T) {
	// Start direct block preparation behind a physical transaction gate.
	v, raw := newPublicationTestVolume(t)
	gate := raw.arm(t)
	putDone := make(chan error, 1)
	go func() {
		_, _, err := v.PrepareOwnedBlock(t.Context(), "direct", []byte("pending construction"), nil)
		putDone <- err
	}()

	// Wait for the direct preparation transaction to reach the gate.
	select {
	case <-gate.started:
	case <-time.After(time.Second):
		t.Fatal("direct transaction did not start")
	}

	// Verify Volume close waits for the blocked direct transaction.
	closed := make(chan error, 1)
	go func() { closed <- v.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close abandoned direct transaction: %v", err)
	case <-time.After(15 * time.Millisecond):
	}

	// Release preparation and verify both preparation and close finish.
	gate.unblock()
	for _, done := range []<-chan error{putDone, closed} {
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(3 * time.Second):
			t.Fatal("direct close did not join")
		}
	}
}
