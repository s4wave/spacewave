package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// callerTurn is the Turn of an engine without an Authority. The engine's
// caller owns the head and holds its blocks, so the turn proposes after the
// published head and accepts every candidate at once, storing its entries
// with no root.
type callerTurn struct {
	// e is the engine.
	e *Engine
}

// Base returns the published head.
func (t *callerTurn) Base() world.Head {
	// A closed engine has no head to build on.
	p := t.e.pub.Load()
	if p == nil {
		return world.Head{}
	}
	return world.Head{Root: p.ref.GetRootRef().Clone(), Seqno: p.seqno, Durable: true}
}

// Admit stores the candidate's entries and accepts it. In deferred mode the
// entries stay in the engine's buffer until Sync; otherwise the store's fence
// makes them durable and the root's completion proof is recorded.
func (t *callerTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	// Store the entries the commit wrote.
	store := t.e.writeBlockStore
	if len(candidate.Entries) != 0 {
		if _, err := store.PutBlockBatch(ctx, candidate.Entries); err != nil {
			return nil, err
		}
	}

	// Fence them durable unless the engine defers durability.
	durable := !t.e.deferredDurability()
	if durable {
		if _, err := store.Sync(ctx); err != nil {
			return nil, err
		}
		if err := block.MarkDerivedRootsComplete(ctx, store, candidate.Base, candidate.Root); err != nil {
			return nil, err
		}
	}
	head := world.Head{Root: candidate.Root.Clone(), Seqno: candidate.Seqno, Durable: durable}
	return &decidedAdmission{seqno: candidate.Seqno, decision: world.Accepted, head: head}, nil
}

// Release does nothing: the turn holds nothing beyond the engine's writer slot.
func (t *callerTurn) Release() {}

// _ is a type assertion
var _ world.Turn = (*callerTurn)(nil)
