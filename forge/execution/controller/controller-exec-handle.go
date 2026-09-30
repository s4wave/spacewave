package execution_controller

import (
	"context"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_transaction "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// execControllerHandle implements ExecControllerHandle from target.
type execControllerHandle struct {
	// ctx is the execution lifecycle that revokes this handle on cancellation.
	ctx context.Context
	// c provides the Execution identity, peer, and claim ID.
	c *Controller
	// ws is the World state used for claim-fenced Execution writes.
	ws world.WorldState
	// ts is the immutable timestamp of the Execution snapshot.
	ts *timestamp.Timestamp
	// claimEpoch is the granted snapshot's epoch, never a later claim's epoch.
	claimEpoch uint64
}

// newExecControllerHandle constructs an ExecControllerHandle.
// ts cannot be nil. claimEpoch comes from the granted Execution snapshot.
func newExecControllerHandle(
	ctx context.Context,
	c *Controller,
	ws world.WorldState,
	ts *timestamp.Timestamp,
	claimEpoch uint64,
) *execControllerHandle {
	return &execControllerHandle{
		ctx:        ctx,
		c:          c,
		ws:         ws,
		ts:         ts,
		claimEpoch: claimEpoch,
	}
}

// GetExecutionUniqueId returns a unique identifier for the execution pass.
func (h *execControllerHandle) GetExecutionUniqueId() string {
	return h.c.uniqueID
}

// GetExecutionObjectKey returns the durable Execution attempt key.
func (h *execControllerHandle) GetExecutionObjectKey() string {
	return h.c.conf.GetObjectKey()
}

// GetExecutionClaimEpoch returns the epoch granted to this handle.
func (h *execControllerHandle) GetExecutionClaimEpoch() uint64 {
	return h.claimEpoch
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
	// Reject storage access after the Execution or request was canceled.
	select {
	case <-h.ctx.Done():
		return h.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Use the World state already granted to this Execution.
	return h.ws.AccessWorldState(ctx, ref, cb)
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
	// Reject output writes after the Execution or request was canceled.
	select {
	case <-h.ctx.Done():
		return h.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Retain the Execution object for the claim-fenced output write.
	obj, err := world.MustGetObject(ctx, h.ws, h.c.conf.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}

	// Build the output write with this handle's granted claim epoch.
	tx, err := execution_transaction.NewTxSetOutputs(
		outps,
		clearOld,
		&forge_execution.Claim{
			ClaimId: h.c.claimID,
			Epoch:   h.claimEpoch,
		},
	)
	if err != nil {
		return err
	}

	// Apply the output transaction as the Execution's authenticated peer.
	_, _, err = obj.ApplyObjectOp(ctx, tx, h.c.peerID)
	return err
}

// WriteLog appends a log entry to the execution.
func (h *execControllerHandle) WriteLog(ctx context.Context, level, message string) error {
	// Reject log writes after the Execution or request was canceled.
	select {
	case <-h.ctx.Done():
		return h.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Timestamp the Execution log entry at the write request.
	entry := &forge_execution.LogEntry{
		Timestamp: timestamp.Now(),
		Level:     level,
		Message:   message,
	}

	// Retain the Execution object for the claim-fenced log write.
	obj, err := world.MustGetObject(ctx, h.ws, h.c.conf.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}

	// Build the log write with this handle's granted claim epoch.
	tx, err := execution_transaction.NewTxAppendLog(
		[]*forge_execution.LogEntry{entry},
		&forge_execution.Claim{
			ClaimId: h.c.claimID,
			Epoch:   h.claimEpoch,
		},
	)
	if err != nil {
		return err
	}

	// Apply the log transaction as the Execution's authenticated peer.
	_, _, err = obj.ApplyObjectOp(ctx, tx, h.c.peerID)
	return err
}

// SetWaitingPlugin records the plugin load wait on the Execution object.
func (h *execControllerHandle) SetWaitingPlugin(ctx context.Context, pluginID string) error {
	// Reject wait-status writes after the Execution or request was canceled.
	select {
	case <-h.ctx.Done():
		return h.ctx.Err()
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Retain the Execution object for the claim-fenced wait-status write.
	obj, err := world.MustGetObject(ctx, h.ws, h.c.conf.GetObjectKey())
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}

	// Write the wait status using this handle's granted claim epoch.
	tx := execution_transaction.NewTxSetWaitingPlugin(pluginID, &forge_execution.Claim{
		ClaimId: h.c.claimID,
		Epoch:   h.claimEpoch,
	})
	_, _, err = obj.ApplyObjectOp(ctx, tx, h.c.peerID)
	return err
}

// _ verifies the execution handle contract.
var _ forge_target.ExecControllerHandle = (*execControllerHandle)(nil)
