package block_gc

import (
	"context"
	"runtime/trace"

	"github.com/pkg/errors"
)

// SweepTarget provides backend-specific deletion for swept nodes.
type SweepTarget interface {
	// DeleteBlock deletes a block from the block store by its IRI.
	DeleteBlock(ctx context.Context, iri string) error
	// DeleteObject deletes an object key from the object store by its IRI.
	DeleteObject(ctx context.Context, iri string) error
}

// WALReplayFunc reads and applies WAL entries to the graph, returning the
// count of entries processed. Entries should be deleted eagerly after apply.
type WALReplayFunc func(ctx context.Context, graph CollectorGraph) (int, error)

// STWLockFunc acquires the STW lock in exclusive mode and returns a
// release function.
type STWLockFunc func() (release func(), err error)

// SweepConfig holds the dependencies for a sweep cycle.
type SweepConfig struct {
	Graph      CollectorGraph
	Target     SweepTarget
	ReplayWAL  WALReplayFunc
	AcquireSTW STWLockFunc
}

// SweepResult holds statistics from a completed sweep cycle.
type SweepResult struct {
	WALEntriesPhase1 int
	WALEntriesPhase2 int
	SweepCandidates  int
	Rescued          int
	Swept            int
}

// SweepCycle runs the full two-phase GC sweep protocol.
//
// Phase 1 (no lock): replay WAL entries into the graph, then run the
// tri-color marker to get sweep candidates.
//
// Phase 2 (exclusive STW lock): process remaining WAL entries with
// transitive rescue, then delete sweep candidates from the backend.
func SweepCycle(ctx context.Context, cfg SweepConfig) (*SweepResult, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/sweep-cycle")
	defer task.End()

	result := &SweepResult{}

	// Phase 1: WAL replay + mark (no lock held).
	phase1Ctx, phase1Task := trace.NewTask(ctx, "hydra/block-gc/sweep-cycle/phase1")
	n, err := cfg.ReplayWAL(phase1Ctx, cfg.Graph)
	phase1Task.End()
	if err != nil {
		return nil, errors.Wrap(err, "phase 1 WAL replay")
	}
	result.WALEntriesPhase1 = n

	marker := NewMarker(cfg.Graph)
	candidates, _, err := marker.Mark(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "phase 1 mark")
	}
	result.SweepCandidates = len(candidates)

	// Build will-delete set for fast lookup.
	willDelete := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		willDelete[c] = true
	}

	// Phase 2: STW reconciliation (exclusive lock).
	phase2Ctx, phase2Task := trace.NewTask(ctx, "hydra/block-gc/sweep-cycle/phase2-stw")
	defer phase2Task.End()

	stwRelease, err := cfg.AcquireSTW()
	if err != nil {
		return nil, errors.Wrap(err, "acquire STW exclusive lock")
	}
	defer stwRelease()

	// 2a: Process remaining WAL entries.
	n2, err := cfg.ReplayWAL(phase2Ctx, cfg.Graph)
	if err != nil {
		return nil, errors.Wrap(err, "phase 2 WAL replay")
	}
	result.WALEntriesPhase2 = n2

	// 2b: Transitive rescue for newly reachable nodes.
	// After replaying Phase 2 WAL entries, re-check each candidate:
	// if it now has incoming refs (besides unreferenced), rescue it
	// and its descendants.
	rescued := 0
	for _, c := range candidates {
		if !willDelete[c] {
			continue
		}
		has, err := cfg.Graph.HasIncomingRefs(phase2Ctx, c)
		if err != nil {
			return nil, errors.Wrap(err, "check rescue")
		}
		if has {
			rescued += transitiveRescue(phase2Ctx, cfg.Graph, c, willDelete)
		}
	}
	result.Rescued = rescued

	// 2c: Execute sweep. Delete-first ordering for crash safety.
	sweepCtx, sweepTask := trace.NewTask(phase2Ctx, "hydra/block-gc/sweep-cycle/sweep")
	swept := 0
	for node := range willDelete {
		if err := sweepNode(sweepCtx, cfg, node); err != nil {
			sweepTask.End()
			return nil, errors.Wrap(err, "sweep "+node)
		}
		swept++
	}
	sweepTask.End()
	result.Swept = swept

	return result, nil
}

// transitiveRescue removes a node and its descendants from the willDelete
// set. Returns the count of nodes rescued.
func transitiveRescue(ctx context.Context, graph CollectorGraph, node string, willDelete map[string]bool) int {
	if !willDelete[node] {
		return 0
	}
	delete(willDelete, node)
	rescued := 1

	targets, err := graph.GetOutgoingRefs(ctx, node)
	if err != nil {
		return rescued
	}
	for _, t := range targets {
		if willDelete[t] {
			rescued += transitiveRescue(ctx, graph, t, willDelete)
		}
	}
	return rescued
}

// sweepNode performs the delete-first, graph-cleanup-second sweep for
// a single node.
func sweepNode(ctx context.Context, cfg SweepConfig, node string) error {
	// Step 1: Backend physical delete first.
	if ref, ok := ParseBlockIRI(node); ok {
		if err := cfg.Target.DeleteBlock(ctx, BlockIRI(ref)); err != nil {
			return errors.Wrap(err, "delete block")
		}
	} else if _, ok := parseObjectIRI(node); ok {
		if err := cfg.Target.DeleteObject(ctx, node); err != nil {
			return errors.Wrap(err, "delete object")
		}
	}

	// Step 2: Remove outgoing edges.
	if _, err := cfg.Graph.RemoveNodeRefs(ctx, node, false); err != nil {
		return errors.Wrap(err, "remove outgoing refs")
	}

	// Step 3: Remove incoming edges to this node.
	incoming, err := cfg.Graph.GetIncomingRefs(ctx, node)
	if err != nil {
		return errors.Wrap(err, "get incoming refs")
	}
	for _, src := range incoming {
		if err := cfg.Graph.RemoveRef(ctx, src, node); err != nil {
			return errors.Wrap(err, "remove incoming ref")
		}
	}

	// Step 4: Remove from root set and node inventory.
	// These are collector-graph-level operations. We call them if the
	// graph supports them (the interface check is done at compile time
	// via CollectorGraph, but RemoveRoot/RemoveNode are separate methods).
	type rootRemover interface {
		RemoveRoot(ctx context.Context, iri string) error
	}
	type nodeRemover interface {
		RemoveNode(ctx context.Context, iri string) error
	}
	if rr, ok := cfg.Graph.(rootRemover); ok {
		_ = rr.RemoveRoot(ctx, node)
	}
	if nr, ok := cfg.Graph.(nodeRemover); ok {
		_ = nr.RemoveNode(ctx, node)
	}

	return nil
}
