package engine

import (
	"context"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// ReplayTarget drives a format 3 engine and its block store as one workload
// replay target.
type ReplayTarget struct {
	*Engine
	*BlockStore
}

// _ is a type assertion
var _ workload.Target = ReplayTarget{}

// AppendJournal journals reference changes through the engine.
func (t ReplayTarget) AppendJournal(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	return t.Append(ctx, adds, removes)
}

// ReplayJournal passes every journaled change to apply through the engine's
// write-ahead log replay.
func (t ReplayTarget) ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error {
	_, err := t.ReplayWAL(ctx, journalGraph{apply: apply})
	return err
}

// journalGraph passes replayed reference changes to a journal apply function.
type journalGraph struct {
	block_gc.CollectorGraph
	// apply receives each replayed batch.
	apply func(adds, removes []block_gc.RefEdge) error
}

// ApplyRefBatch passes the changes to apply.
func (g journalGraph) ApplyRefBatch(_ context.Context, adds, removes []block_gc.RefEdge) error {
	return g.apply(adds, removes)
}
