package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// callerTurn is the Turn of an engine without an Authority. The engine's
// caller owns the head, so the turn proposes after the published head and
// accepts every candidate at once, storing its entries with no root under the
// engine's stage, which holds them until Close.
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
	return world.Head{Root: p.rootRef().Clone(), Seqno: p.seqno, Durable: true}
}

// Admit stores the candidate's entries and accepts it. In deferred mode the
// entries stay in the engine's buffer until Sync. Otherwise the store's fence
// makes them durable and the root's completion proof is recorded.
func (t *callerTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	// Store the entries the commit wrote under the engine's stage. In
	// deferred mode writeBlockStore is that stage.
	store := t.e.writeBlockStore
	durable := !t.e.deferredDurability()
	if len(candidate.Entries) != 0 {
		staged := store
		if durable {
			var err error
			if staged, err = t.e.openCallerStage(ctx); err != nil {
				return nil, err
			}
		}
		if _, err := staged.PutBlockBatch(ctx, candidate.Entries); err != nil {
			return nil, err
		}
	}

	// Fence them durable unless the engine defers durability.
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

// openCallerStage returns the stage a durable commit of an engine without an
// Authority stores through, opening it on first use.
func (e *Engine) openCallerStage(ctx context.Context) (block.StoreOps, error) {
	// Reuse the stage an earlier commit opened.
	locked := e.bcast.Lock()
	staged := e.callerStage
	locked.Unlock()
	if staged != nil {
		return staged, nil
	}

	// Open one, and keep the first a concurrent commit installs.
	staged, release, err := block.OpenStage(ctx, e.writeBlockStore)
	if err != nil {
		return nil, err
	}
	locked = e.bcast.Lock()
	switch {
	case e.closed:
		err = e.closedErr()
	case e.callerStage == nil:
		e.callerStage, e.releaseStage, release = staged, release, nil
	default:
		staged = e.callerStage
	}
	locked.Unlock()
	if release != nil {
		release()
	}
	return staged, err
}

// Release does nothing: the turn holds nothing beyond the engine's writer slot.
func (t *callerTurn) Release() {}

// WaitBase returns at once: the published head is durable.
func (t *callerTurn) WaitBase(context.Context) error {
	return nil
}

// _ is a type assertion
var _ world.Turn = (*callerTurn)(nil)
