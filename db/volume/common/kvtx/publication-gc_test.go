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
	t.Helper()
	node := block_gc.BlockIRI(p.Entries[0].Ref)
	if err := v.GetRefGraph().ApplyRefBatch(t.Context(), nil, []block_gc.RefEdge{{Subject: block_gc.BucketIRI(p.BucketID), Object: node}}); err != nil {
		t.Fatal(err)
	}
	candidates, err := v.GetRefGraph().GetUnreferencedNodes(t.Context())
	if err != nil || !slices.Contains(candidates, node) {
		t.Fatalf("candidate %q missing: %v %v", node, candidates, err)
	}
	return node
}

func TestPublicationSweepRechecksRescuedCandidate(t *testing.T) {
	v, _ := newPublicationTestVolume(t)
	p := publicationFor(t, "sweep", "", "one")
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	node := orphanPublication(t, v, p)
	// The collector already holds node in a candidate snapshot. Publication
	// rescues the same bytes before the physical deletion can begin.
	p.Head = publicationFor(t, "sweep", "one", "two").Head
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	removed, err := v.SweepUnreferenced(t.Context(), v.GetRefGraph(), []string{node})
	if err != nil || len(removed) != 0 {
		t.Fatalf("rescued candidate: removed=%v err=%v", removed, err)
	}
	assertPublishedBlock(t, v, p, true)
	assertHead(t, v, "sweep", "two")
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
	v, _ := newPublicationTestVolume(t)
	p := publicationFor(t, "collector", "", "one")
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	orphanPublication(t, v, p)
	store := &rescueSweepStore{Volume: v, rescue: func() error {
		return v.PrepareOwnedBlockBatch(t.Context(), p.BucketID, p.Entries)
	}}
	stats, err := block_gc.NewCollector(v.GetRefGraph(), store, nil).Collect(t.Context())
	if err != nil || stats.NodesSwept != 0 || stats.AtomicSweepCount == 0 {
		t.Fatalf("collector stats=%+v err=%v", stats, err)
	}
	assertPublishedBlock(t, v, p, true)
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
	raw.mu.Lock()
	before := raw.commits
	raw.mu.Unlock()
	stats, err := block_gc.NewCollector(v.GetRefGraph(), v, nil).Collect(t.Context())
	if err != nil || stats.NodesSwept != len(published) || stats.RemoveBlockCount != len(published) {
		t.Fatalf("collector stats=%+v err=%v", stats, err)
	}
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
	v, raw := newPublicationTestVolume(t)
	p := publicationFor(t, "sweep-failure", "", "one")
	if err := v.PublishAtomic(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	node := orphanPublication(t, v, p)
	failure := errors.New("sync failed")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()
	removed, err := v.SweepUnreferenced(t.Context(), v.GetRefGraph(), []string{node})
	if len(removed) != 0 || !errors.Is(err, failure) {
		t.Fatalf("failed sweep: %v %v", removed, err)
	}
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()
	data, found, err := v.GetBlock(t.Context(), p.Entries[0].Ref)
	if err != nil || !found || !bytes.Equal(data, p.Entries[0].Data) {
		t.Fatal("failed sweep deleted bytes", err)
	}
	incoming, err := v.GetRefGraph().GetIncomingRefs(t.Context(), node)
	if err != nil || !slices.Contains(incoming, block_gc.NodeUnreferenced) {
		t.Fatal("failed sweep deleted marker", incoming, err)
	}
	removed, err = v.SweepUnreferenced(t.Context(), v.GetRefGraph(), []string{node})
	if len(removed) != 1 || err != nil {
		t.Fatalf("retry sweep: %v %v", removed, err)
	}
}

func TestPublicationPreparationAtomicAndOversized(t *testing.T) {
	v, raw := newPublicationTestVolume(t)
	// Larger than the World staging cap: direct preparation is not admitted
	// into the bounded publication queue and cannot publish a head.
	data := bytes.Repeat([]byte("large-body"), (5<<20)/10)
	raw.mu.Lock()
	before := raw.commits
	raw.mu.Unlock()
	ref, existed, err := v.PrepareOwnedBlock(t.Context(), "large", data, nil)
	if err != nil || existed {
		t.Fatalf("prepare: %v %v", existed, err)
	}
	raw.mu.Lock()
	count := raw.commits - before
	raw.mu.Unlock()
	if count != 1 {
		t.Fatalf("preparation used %d physical commits", count)
	}
	p := &block.AtomicPublication{BucketID: "large", Entries: []*block.PutBatchEntry{{Ref: ref, Data: data}}}
	assertPublishedBlock(t, v, p, true)
	assertHead(t, v, "large", "")
	if stats := v.GetPublicationStats(); stats.Accepted != 0 {
		t.Fatalf("preparation was queued: %+v", stats)
	}
	_, existed, err = v.PrepareOwnedBlock(t.Context(), "large", data, nil)
	if err != nil || !existed {
		t.Fatalf("duplicate preparation: %v %v", existed, err)
	}
	failure := errors.New("preparation commit failed")
	failed := publicationFor(t, "unpublished", "", "one")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()
	err = v.PrepareOwnedBlockBatch(t.Context(), failed.BucketID, failed.Entries)
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	assertPublishedBlock(t, v, failed, false)
}

func TestPublicationDirectCloseAndGraphScope(t *testing.T) {
	v, _ := newPublicationTestVolume(t)
	other, _ := newPublicationTestVolume(t)
	removed, err := v.SweepUnreferenced(t.Context(), other.GetRefGraph(), []string{"object:wrong-scope"})
	if len(removed) != 0 || !errors.Is(err, block_gc.ErrAtomicSweepUnsupported) {
		t.Fatalf("wrong scope: %v %v", removed, err)
	}
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	_, _, err = v.PrepareOwnedBlock(t.Context(), "closed", []byte("data"), nil)
	if !errors.Is(err, block.ErrPublicationClosed) {
		t.Fatal("preparation admitted after close", err)
	}
}

func TestPublicationGraphFailureRetainsWholeTransition(t *testing.T) {
	v, raw := newPublicationTestVolume(t)
	adds := make([]block_gc.RefEdge, 4100) // crosses RefGraph's virtual slice boundary
	for i := range adds {
		adds[i] = block_gc.RefEdge{Subject: "retry-owner", Object: fmt.Sprintf("object:retry-%d", i)}
	}
	failure := errors.New("outer graph commit failed")
	raw.mu.Lock()
	raw.errorCommit = failure
	raw.mu.Unlock()
	err := v.GetRefGraph().ApplyRefBatch(t.Context(), adds, nil)
	raw.mu.Lock()
	raw.errorCommit = nil
	raw.mu.Unlock()
	ra, rr, ok := block_gc.RefBatchRemainder(err)
	if !errors.Is(err, failure) || !ok || len(ra) != len(adds) || len(rr) != 0 {
		t.Fatalf("lost retry prefix: %v adds=%d removes=%d", err, len(ra), len(rr))
	}
	got, err := v.GetRefGraph().GetOutgoingRefs(t.Context(), "retry-owner")
	if err != nil || len(got) != 0 {
		t.Fatalf("failed graph leaked edges: %d %v", len(got), err)
	}
	if err := v.GetRefGraph().ApplyRefBatch(t.Context(), ra, rr); err != nil {
		t.Fatal(err)
	}
	got, err = v.GetRefGraph().GetOutgoingRefs(t.Context(), "retry-owner")
	if err != nil || len(got) != len(adds) {
		t.Fatalf("retry edges=%d want=%d: %v", len(got), len(adds), err)
	}
}

func TestPublicationCloseJoinsDirectPreparation(t *testing.T) {
	v, raw := newPublicationTestVolume(t)
	gate := raw.arm(t)
	putDone := make(chan error, 1)
	go func() {
		_, _, err := v.PrepareOwnedBlock(t.Context(), "direct", []byte("pending construction"), nil)
		putDone <- err
	}()
	select {
	case <-gate.started:
	case <-time.After(time.Second):
		t.Fatal("direct transaction did not start")
	}
	closed := make(chan error, 1)
	go func() { closed <- v.Close() }()
	select {
	case err := <-closed:
		t.Fatalf("Close abandoned direct transaction: %v", err)
	case <-time.After(15 * time.Millisecond):
	}
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
