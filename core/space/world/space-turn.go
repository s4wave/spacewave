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
//
// Invariant: every block Admit puts to the inner store is held, by a stage or
// a root, from before the put until a stored World names it. The blocks past
// the PendingStore's bound go to a stage opened before their put, and the
// stage is released only after holdAdmitted moved the pending root onto the
// candidate, which reaches them; the pending root holds them until a
// checkpoint or cursor save stores a World that names them.
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
	payloads, err := tx.PayloadRefs(ctx, a.lookupOp)
	if err != nil {
		return nil, err
	}

	// Admit it before keeping its blocks, so a sweep on the driver goroutine
	// finds its root among the pending candidates once the blocks are kept.
	adm := a.newAdmission(candidate, op, payloads)
	a.admit(adm)
	releaseStage, err := a.pages.PutBlockBatchStaged(ctx, candidate.Entries)
	if err == nil {
		err = admitStep(admitStepPut)
	}
	if err != nil {
		if releaseStage != nil {
			releaseStage()
		}
		a.fail(adm, err)
		return nil, err
	}

	// Hold what the queued operation needs durably before queueing it: its
	// payloads, which name every object root its apply set (Tx.PayloadRefs).
	// The hold stores each kept page a root reaches, nested refs included, in
	// one transaction with the root's move, so an appended operation never
	// names a block the store may drop. A crash after the hold and before the
	// append leaves only the hold, which the next resetPayloads drops.
	err = a.holdAdmitted(ctx, adm, releaseStage != nil)
	if err == nil {
		err = admitStep(admitStepHeld)
	}
	if releaseStage != nil {
		releaseStage()
	}
	if err != nil {
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

// admitStepPut and admitStepHeld name the points of Admit a test may stop at:
// after the put, and after the hold before the stage is released.
const (
	admitStepPut  = "put"
	admitStepHeld = "held"
)

// admitStep is a test hook run at each named point of Admit. A returned error
// fails the admission there, as a crash at that point would leave it.
var admitStep = func(string) error { return nil }

// _ is a type assertion
var _ world.Turn = (*spaceTurn)(nil)
