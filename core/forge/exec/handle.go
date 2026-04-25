package space_exec

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

// execHandle implements ExecControllerHandle without bus access.
// Applies transactions directly to the execution object via world state.
type execHandle struct {
	ctx       context.Context
	ws        world.WorldState
	objectKey string
	peerID    peer.ID
	uniqueID  string
	ts        *timestamp.Timestamp
}

// newExecHandle constructs a space-aware ExecControllerHandle.
func newExecHandle(
	ctx context.Context,
	ws world.WorldState,
	objectKey string,
	peerID peer.ID,
	uniqueID string,
	ts *timestamp.Timestamp,
) *execHandle {
	return &execHandle{
		ctx:       ctx,
		ws:        ws,
		objectKey: objectKey,
		peerID:    peerID,
		uniqueID:  uniqueID,
		ts:        ts,
	}
}

// GetExecutionUniqueId returns a unique identifier for the execution.
func (h *execHandle) GetExecutionUniqueId() string {
	return h.uniqueID
}

// GetPeerId returns the executing peer ID.
func (h *execHandle) GetPeerId() peer.ID {
	return h.peerID
}

// GetTimestamp returns the execution timestamp.
func (h *execHandle) GetTimestamp() *timestamp.Timestamp {
	return h.ts
}

// AccessStorage builds a read-only bucket lookup cursor at the given ref.
func (h *execHandle) AccessStorage(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	if err := h.ctx.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return h.ws.AccessWorldState(ctx, ref, cb)
}

// SetOutputs updates execution outputs via world op transaction.
func (h *execHandle) SetOutputs(
	ctx context.Context,
	outps forge_value.ValueSlice,
	clearOld bool,
) error {
	if err := h.ctx.Err(); err != nil {
		return err
	}
	obj, err := world.MustGetObject(ctx, h.ws, h.objectKey)
	if err != nil {
		return err
	}
	tx, err := execution_transaction.NewTxSetOutputs(outps, clearOld)
	if err != nil {
		return err
	}
	_, _, err = obj.ApplyObjectOp(ctx, tx, h.peerID)
	return err
}

// WriteLog appends a log entry to the execution.
func (h *execHandle) WriteLog(ctx context.Context, level, message string) error {
	if err := h.ctx.Err(); err != nil {
		return err
	}
	entry := &forge_execution.LogEntry{
		Timestamp: timestamp.Now(),
		Level:     level,
		Message:   message,
	}
	obj, err := world.MustGetObject(ctx, h.ws, h.objectKey)
	if err != nil {
		return err
	}
	tx, err := execution_transaction.NewTxAppendLog([]*forge_execution.LogEntry{entry})
	if err != nil {
		return err
	}
	_, _, err = obj.ApplyObjectOp(ctx, tx, h.peerID)
	return err
}

// _ is a type assertion
var _ forge_target.ExecControllerHandle = (*execHandle)(nil)
