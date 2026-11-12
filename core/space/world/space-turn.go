package space_world

import (
	"context"
	"sync/atomic"

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
// Invariant: every block the candidate's writer put to the inner store is
// held, by a stage or a root, from before the put until a stored World names
// it. Admit's own put past the PendingStore's bound and the writer's
// transaction, which may drain early, both put through stages that are
// released only after Admit returns, and while any stage that stored is open
// holdAdmitted moves the pending root onto the candidate, which reaches what
// they stored; the pending root holds it until a checkpoint or cursor save
// stores a World that names it.
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
	if err := a.storeAdmitted(ctx, adm); err != nil {
		a.fail(adm, err)
		return nil, err
	}
	a.queue.push(adm)
	return adm, nil
}

// storeAdmitted keeps adm's blocks in the PendingStore, puts those past its
// bound under a stage, and holds what the queued operation needs durably
// before it is queued: its payloads, which name every object root its apply
// sets (Tx.PayloadRefs), and its World while a stage stored part of it. The
// hold stores each kept page a root reaches, nested refs included, in one
// transaction with the root's move, so an appended operation never names a
// block the store may drop. A crash after the hold and before the append
// leaves only the hold, which the next resetPayloads drops.
func (a *Adapter) storeAdmitted(ctx context.Context, adm *spaceAdmission) error {
	// Keep the blocks, and put the rest under a stage held through the hold.
	releaseStage, err := a.pages.PutBlockBatchStaged(ctx, adm.candidate.Entries)
	if err != nil {
		return err
	}
	err = admitStep(admitStepPut)

	// Hold the payloads and the World, then end the stage.
	if err == nil {
		err = a.holdAdmitted(ctx, adm)
	}
	if err == nil {
		err = admitStep(admitStepHeld)
	}
	if releaseStage != nil {
		releaseStage()
	}
	if err != nil {
		return err
	}
	return admitStep(admitStepReleased)
}

// Release unlocks the turn, so the next writer begins on this turn's
// candidate.
func (t *spaceTurn) Release() {
	t.release()
}

// admitStepPut, admitStepHeld and admitStepReleased name the points of Admit
// a test may stop at: after the put, after the hold before the stage is
// released, and after the release before the operation is queued.
const (
	admitStepPut      = "put"
	admitStepHeld     = "held"
	admitStepReleased = "released"
)

// admitStepHook is a test hook run at each named point of Admit, or nil. A
// returned error fails the admission there, as a crash at that point would
// leave it.
var admitStepHook atomic.Pointer[func(string) error]

// admitStep runs the test hook at step, when one is set.
func admitStep(step string) error {
	if fn := admitStepHook.Load(); fn != nil {
		return (*fn)(step)
	}
	return nil
}

// _ is a type assertion
var _ world.Turn = (*spaceTurn)(nil)
