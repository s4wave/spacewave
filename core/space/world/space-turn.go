package space_world

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// spaceTurn is one local writer's turn on a Space World. It holds the
// adapter's turn mutex from Begin to Release, and its base is the writer head
// when it began.
type spaceTurn struct {
	// a is the adapter.
	a *Adapter
	// base is the writer head the turn builds on.
	base world.Head
	// release unlocks the adapter's turn mutex once.
	release func()
}

// Base returns the writer head when the turn began.
func (t *spaceTurn) Base() world.Head {
	return t.base
}

// Admit registers the candidate, keeps its blocks in the adapter's
// PendingStore, holds its payloads, makes its root the writer head and
// appends its operation to the queue. It returns before the operation is
// queued, with an Admission that waits for the driver to apply it.
func (t *spaceTurn) Admit(ctx context.Context, candidate world.Candidate) (world.Admission, error) {
	// Wrap the transaction batch in the Space operation.
	a := t.a
	if len(candidate.Operation) == 0 {
		return nil, errors.New("space world candidate carries no operation")
	}
	tx := &world_block_tx.Tx{}
	if err := tx.UnmarshalVT(candidate.Operation); err != nil {
		return nil, err
	}
	op, err := (&WorldOp{Body: &WorldOp_ApplyTxOp{ApplyTxOp: &ApplyTxOp{Tx: tx}}}).MarshalVT()
	if err != nil {
		return nil, err
	}

	// Read the payload roots it names and the handlers that name them.
	payloads, err := tx.PayloadRefs(ctx, a.lookupOp)
	if err != nil {
		return nil, err
	}
	handlers, err := a.payloadHandlers(ctx, tx)
	if err != nil {
		return nil, err
	}

	// Admit it before keeping its blocks, so a sweep on the driver goroutine
	// finds its root among the pending candidates once the blocks are kept.
	adm := a.newAdmission(candidate, op, payloads, handlers)
	a.admit(adm)
	if _, err := a.pages.PutBlockBatch(ctx, candidate.Entries); err != nil {
		a.fail(adm, err)
		return nil, err
	}

	// Invariant: when the commit of an operation returns, every block under
	// the roots it names is durable in the Space's block store on this
	// device; a crash at any point leaves either no operation, or the
	// operation with all its blocks. Order: blocks durable first, then the
	// operation. The roots include a World stage's build, which its stage
	// may still keep in memory: hold the payloads, which name every object
	// root the apply set (Tx.PayloadRefs), before queueing the operation. The
	// hold stores each kept page a root reaches, nested refs included, in
	// one transaction with the root's move; a builder that called
	// WorldStage.StoreRoots left none to store here. A crash after the hold
	// and before the append leaves only the hold, which the next
	// resetPayloads drops.
	if err := a.holdAdmitted(ctx, adm); err != nil {
		a.fail(adm, err)
		return nil, err
	}
	a.queue.push(adm)
	return adm, nil
}

// Release unlocks the turn, so the next writer begins on this turn's
// candidate.
func (t *spaceTurn) Release() {
	t.release()
}

// _ is a type assertion
var _ world.Turn = (*spaceTurn)(nil)
