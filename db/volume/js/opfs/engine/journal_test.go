//go:build !js

package engine

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume/js/opfs/refgraph"
)

// journalSweepTarget deletes swept block IRIs from the durable pack store.
type journalSweepTarget struct {
	// store owns the durable block locations removed during sweep.
	store *packStore
}

// DeleteBlock removes one parsed block from durable storage.
func (t *journalSweepTarget) DeleteBlock(ctx context.Context, iri string) error {
	ref, ok := block_gc.ParseBlockIRI(iri)
	if !ok {
		return nil
	}
	return t.store.RmBlock(ctx, ref)
}

// DeleteObject leaves object deletion outside this block-only fixture.
func (t *journalSweepTarget) DeleteObject(context.Context, string) error { return nil }

// TestJournalSweepRescuesReferenceAppendedBetweenReplays proves the two-phase fence.
func TestJournalSweepRescuesReferenceAppendedBetweenReplays(t *testing.T) {
	// Open a durable engine for the journal sweep race.
	ctx := t.Context()
	engine, err := Open(ctx, newDiskBackend(t))
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()

	// Publish reachable, doomed, and rescue-candidate blocks.
	store := &packStore{engine: engine}
	put := func(data string) *block.BlockRef {
		t.Helper()
		ref, existed, err := store.PutBlock(ctx, []byte(data), &block.PutOpts{Sync: true})
		if err != nil || existed {
			t.Fatalf("put %q: %t %v", data, existed, err)
		}
		return ref
	}
	reachableRef := put("reachable block")
	doomedRef := put("unreferenced block")
	rescuedRef := put("rescued block")

	// Journal the initial reference graph before sweeping.
	reachable := block_gc.BlockIRI(reachableRef)
	doomed := block_gc.BlockIRI(doomedRef)
	rescued := block_gc.BlockIRI(rescuedRef)
	if err := engine.Append(ctx, []block_gc.RefEdge{
		{Subject: block_gc.NodeGCRoot, Object: reachable},
		{Subject: doomed, Object: reachable},
		{Subject: rescued, Object: reachable},
	}, nil); err != nil {
		t.Fatal(err)
	}

	// Run a sweep that appends a rescue reference between its replay phases.
	graph := refgraph.NewGraph(engine)
	replayCount := 0
	replay := func(ctx context.Context, graph block_gc.CollectorGraph) (int, error) {
		// Replay the current journal before injecting the rescue reference.
		count, err := engine.ReplayWAL(ctx, graph)
		if err != nil {
			return count, err
		}
		replayCount++
		if replayCount == 1 {
			err = engine.Append(ctx, []block_gc.RefEdge{{
				Subject: block_gc.NodeGCRoot,
				Object:  rescued,
			}}, nil)
		}
		return count, err
	}
	result, err := block_gc.SweepCycle(ctx, block_gc.SweepConfig{
		Graph:     graph,
		Target:    &journalSweepTarget{store: store},
		ReplayWAL: replay,
		AcquireSTW: func() (func(), error) {
			return engine.AcquireSTW(ctx)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.WALEntriesPhase1 != 1 || result.WALEntriesPhase2 != 1 ||
		result.SweepCandidates != 2 || result.Rescued != 1 || result.Swept != 1 {
		t.Fatalf("sweep result = %+v, want one rescued and one swept candidate", result)
	}

	// Verify surviving block membership matches reachability and rescue.
	for _, check := range []struct {
		name string
		ref  *block.BlockRef
		want bool
	}{
		{name: "reachable", ref: reachableRef, want: true},
		{name: "unreferenced", ref: doomedRef, want: false},
		{name: "rescued", ref: rescuedRef, want: true},
	} {
		found, err := store.GetBlockExists(ctx, check.ref)
		if err != nil {
			t.Fatalf("check %s block: %v", check.name, err)
		}
		if found != check.want {
			t.Errorf("%s block exists = %t, want %t", check.name, found, check.want)
		}
	}
}
