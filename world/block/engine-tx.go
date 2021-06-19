package world_block

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/tx"
	"github.com/aperturerobotics/hydra/world"
)

// EngineTx is an engine transaction wrapping the Tx object.
// returned by e.NewTransaction
type EngineTx struct {
	rel    uint32
	engine *Engine

	writeTx *Tx
}

// newEngineTx constructs a new EngineTx.
func newEngineTx(e *Engine, writeTx *Tx) *EngineTx {
	return &EngineTx{writeTx: writeTx, engine: e}
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
// If not write, returns ErrNotWrite.
func (e *EngineTx) Commit(ctx context.Context) error {
	if e.writeTx == nil {
		return tx.ErrNotWrite
	}

	// ensure tx is not already discarded
	if !e.release() {
		return tx.ErrDiscarded
	}

	// commit
	commitErr := e.writeTx.Commit(e.engine.ctx)
	var nroot *block.BlockRef

	// validate the new root
	if commitErr == nil {
		nroot = e.writeTx.state.GetRootRef()
		commitErr = nroot.Validate()
	}

	// apply committed changes or rollback
	if commitErr == nil {
		e.engine.rmtx.Lock()
		oldRoot := e.engine.root.GetRef().GetRootRef()
		e.engine.root.SetRootRef(nroot)
		commitErr = e.engine.updateReadTx()
		if commitErr != nil {
			e.engine.root.SetRootRef(oldRoot)
		}
		e.engine.rmtx.Unlock()
	}

	e.engine.wmtx.Unlock()
	return commitErr
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
func (e *EngineTx) Discard() {
	if e.release() {
		if e.writeTx != nil {
			e.writeTx.Discard()
			e.engine.wmtx.Unlock()
		}
	}
}

// release releases the tx
func (e *EngineTx) release() bool {
	rel := atomic.SwapUint32(&e.rel, 1)
	return rel != 1
}

// GetReadOnly returns if the state is read-only.
func (e *EngineTx) GetReadOnly() bool {
	return e.writeTx == nil
}

// GetSeqno returns the current seqno of the world state.
// This is also the sequence number of the most recent change.
// Initializes at 0 for initial world state.
func (e *EngineTx) GetSeqno() (uint64, error) {
	var seqno uint64
	err := e.performOp(func(tx *Tx) error {
		var berr error
		seqno, berr = tx.GetSeqno()
		return berr
	})
	return seqno, err
}

// _ is a type assertion
var _ world.Tx = ((*EngineTx)(nil))
