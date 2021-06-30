package execution_controller

import (
	"context"

	execution_transaction "github.com/aperturerobotics/forge/execution/tx"
	"github.com/aperturerobotics/forge/target"
	"github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
)

// execControllerHandle implements ExecControllerHandle from target.
type execControllerHandle struct {
	ctx context.Context
	c   *Controller
	eng world.Engine
}

// newExecControllerHandle constructs an ExecControllerHandle.
func newExecControllerHandle(ctx context.Context, c *Controller, eng world.Engine) *execControllerHandle {
	return &execControllerHandle{ctx: ctx, c: c, eng: eng}
}

// AccessStorage builds a bucket lookup cursor located at the given ref.
// If the ref is empty, will produce a cursor at the root of the world.
// If the ref Bucket ID is empty, uses the same bucket + volume as the world.
// The cursor returned is read-only.
// The lookup cursor will be released after cb returns.
func (h *execControllerHandle) AccessStorage(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	select {
	case <-h.ctx.Done():
		return h.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	return h.eng.AccessWorldState(ctx, ref, cb)
}

// SetOutputs changes the outputs according to the given ValueSlice.
// Note: the slice contents will be copied before the call returns.
// Note: each Value must be named.
// Returns context.Canceled if the handle ctx is canceled.
func (h *execControllerHandle) SetOutputs(
	ctx context.Context,
	outps forge_value.ValueSlice,
	clearOld bool,
) error {
	select {
	case <-h.ctx.Done():
		return h.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	worldTx, err := h.eng.NewTransaction(true)
	if err != nil {
		return err
	}
	defer worldTx.Discard()

	obj, err := world.MustGetObject(worldTx, h.c.conf.GetObjectId())
	if err != nil {
		return err
	}

	tx, err := execution_transaction.NewTxSetOutputs(outps, clearOld)
	if err != nil {
		return err
	}

	// execution_transaction.ExecutionTxType_EXECUTION_TX_TYPE_SET_OUTPUTS
	_, err = obj.ApplyObjectOp(
		execution_transaction.ObjectOperationTypeID,
		tx,
		h.c.peerID,
	)
	if err != nil {
		return err
	}
	return worldTx.Commit(h.ctx)
}

// _ is a type assertion
var _ forge_target.ExecControllerHandle = ((*execControllerHandle)(nil))
