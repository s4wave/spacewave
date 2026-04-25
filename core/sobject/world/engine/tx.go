package sobject_world_engine

import (
	"context"
	"runtime/trace"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
)

// soEngineWriteTx is the write txn attached to the soEngine
type soEngineWriteTx struct {
	*world_block_tx.WorldState
	btx            *world_block.Tx
	eng            *soEngine
	unlockWriteMtx func()
}

// newSoEngineWriteTx constructs a new shared object engine tx.
func newSoEngineWriteTx(
	worldState *world_block_tx.WorldState,
	btx *world_block.Tx,
	eng *soEngine,
	unlockWriteMtx func(),
) *soEngineWriteTx {
	return &soEngineWriteTx{
		WorldState:     worldState,
		btx:            btx,
		eng:            eng,
		unlockWriteMtx: unlockWriteMtx,
	}
}

// Commit commits the transaction to storage.
// Can return an error to indicate tx failure.
func (t *soEngineWriteTx) Commit(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/commit")
	defer task.End()

	released := false
	release := func() {
		if released {
			return
		}
		released = true
		t.Discard()
	}
	defer release() // discard the underlying block txn and unlock the write mtx

	// commit the upper world state so we can get the txns list
	// world_block_tx.WorldState Commit just checks if discarded & marks as discarded
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/world-state-commit")
		err := t.WorldState.Commit(taskCtx)
		task.End()
		if err != nil {
			return err
		}
	}

	// commit the block txn
	var nroot *block.BlockRef
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/block-commit")
		var err error
		nroot, err = t.btx.CommitBlockTransaction(taskCtx)
		task.End()
		if err != nil {
			return err
		}
	}

	txBatch := t.GetTxBatch()
	txns := txBatch.GetTxs()
	if len(txns) == 0 {
		// no-op
		return nil
	}

	var tx *world_block_tx.Tx
	{
		_, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/build-tx-batch")
		var err error
		tx, err = world_block_tx.NewTxBatch(txBatch)
		task.End()
		if err != nil {
			return err
		}
	}

	// apply world op
	op := &SOWorldOp{
		Body: &SOWorldOp_ApplyTxOp{
			ApplyTxOp: &ApplyTxOp{Tx: tx},
		},
	}

	// marshal op data
	opData, err := op.MarshalVT()
	if err != nil {
		return err
	}

	// queue the operation
	var localOpID string
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/queue-operation")
		var err error
		localOpID, err = t.eng.so.QueueOperation(taskCtx, opData)
		task.End()
		if err != nil {
			return err
		}
	}
	_ = localOpID

	// build the next obj ref
	baseObjRef := t.eng.bengine.GetRootRef() // clone of current (pre-commit) root
	nextObjRef := baseObjRef.CloneVT()
	nextObjRef.RootRef = nroot

	// Cache the commit result for replay adoption. Watch-state and
	// validator can adopt this instead of re-executing processOp when
	// the base root ref and op bytes match.
	{
		cachedHeadRef := nextObjRef.CloneVT()
		cachedHeadRef.BucketId = ""
		t.eng.c.lastCommitResult.Store(&commitResult{
			baseRootRef: baseObjRef.GetRootRef(),
			opData:      opData,
			resultState: &InnerState{HeadRef: cachedHeadRef},
		})
	}

	// Update the local state
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/update-engine-state")
		err := t.eng.updateEngineState(taskCtx, nextObjRef)
		task.End()
		if err != nil {
			return err
		}
	}

	t.eng.c.notifyGCSweepMaintenance()

	release()
	{
		taskCtx, task := trace.NewTask(ctx, "alpha/so-engine/write-tx/wait-operation")
		_, rejected, err := t.eng.so.WaitOperation(taskCtx, localOpID)
		task.End()
		if err != nil {
			if rejected {
				_ = t.eng.so.ClearOperationResult(ctx, localOpID)
			}
			return err
		}
	}

	// done
	return nil
}

// Discard cancels the transaction.
// If called after Commit, does nothing.
// Cannot return an error.
// Can be called unlimited times.
// Always call Discard or Commit when done with a tx.
func (t *soEngineWriteTx) Discard() {
	t.WorldState.Discard()
	t.btx.Discard()
	t.unlockWriteMtx()
}

// _ is a type assertion
var _ world.Tx = (*soEngineWriteTx)(nil)
