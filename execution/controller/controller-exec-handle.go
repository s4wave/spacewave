package execution_controller

import (
	"context"

	"github.com/aperturerobotics/bifrost/peer"
	execution_transaction "github.com/aperturerobotics/forge/execution/tx"
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/world"
	"github.com/aperturerobotics/timestamp"
)

// execControllerHandle implements ExecControllerHandle from target.
type execControllerHandle struct {
	ctx context.Context
	c   *Controller
	ws  world.WorldState
	ts  *timestamp.Timestamp
}

// newExecControllerHandle constructs an ExecControllerHandle.
// ts cannot be nil
func newExecControllerHandle(ctx context.Context, c *Controller, ws world.WorldState, ts *timestamp.Timestamp) *execControllerHandle {
	return &execControllerHandle{ctx: ctx, c: c, ws: ws, ts: ts}
}

// GetExecutionUniqueId returns a unique identifier for the execution pass.
func (h *execControllerHandle) GetExecutionUniqueId() string {
	return h.c.uniqueID
}

// GetPeerId returns the peer id that this exec controller is operating as.
func (h *execControllerHandle) GetPeerId() peer.ID {
	return h.c.peerID
}

// GetTimestamp returns the timestamp for the handle.
func (h *execControllerHandle) GetTimestamp() *timestamp.Timestamp {
	return h.ts
}

// AccessStorage builds a bucket lookup cursor located at the given ref.
// If the ref is empty, will produce a cursor at the root of the target world.
// If the ref Bucket ID is empty, uses the same bucket + volume as the target world.
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

	// TODO: access target world state?
	access := h.ws.AccessWorldState
	return access(ctx, ref, cb)
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

	obj, err := world.MustGetObject(h.ws, h.c.conf.GetObjectKey())
	if err != nil {
		return err
	}

	tx, err := execution_transaction.NewTxSetOutputs(outps, clearOld)
	if err != nil {
		return err
	}

	// execution_transaction.ExecutionTxType_EXECUTION_TX_TYPE_SET_OUTPUTS
	_, _, err = obj.ApplyObjectOp(
		tx,
		h.c.peerID,
	)
	return err
}

// _ is a type assertion
var _ forge_target.ExecControllerHandle = ((*execControllerHandle)(nil))
