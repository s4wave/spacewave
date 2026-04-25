package provider_local

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/core/sobject"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
)

// lsoStateSnapshot wraps SharedObjectStateSnapshot to include local pending operations.
type lsoStateSnapshot struct {
	h *sobject.SOStateParticipantHandle
	// localState contains the current local state
	localState *LocalSOState
}

// newLsoStateSnapshot constructs a new lsoStateSnapshot.
func newLsoStateSnapshot(
	h *sobject.SOStateParticipantHandle,
	localState *LocalSOState,
) *lsoStateSnapshot {
	return &lsoStateSnapshot{
		h:          h,
		localState: localState,
	}
}

// GetOpQueue overrides SharedObjectStateSnapshot.GetOpQueue returning the local queued ops as well.
func (l *lsoStateSnapshot) GetOpQueue(ctx context.Context) ([]*sobject.SOOperation, []*sobject.QueuedSOOperation, error) {
	// Get the underlying op queue
	opQueue, queuedOps, err := l.h.GetOpQueue(ctx)
	if err != nil {
		return nil, nil, err
	}

	// queuedOps is usually nil here.
	// add this to be safe.
	if queuedOps != nil {
		queuedOps = slices.Clone(queuedOps)
	}

	// Add locally pending operations
	queuedOps = append(queuedOps, l.localState.GetOpQueue()...)

	return opQueue, queuedOps, nil
}

// --- pass through functions ---

// GetParticipantConfig returns the participant record for our participant.
func (l *lsoStateSnapshot) GetParticipantConfig(ctx context.Context) (*sobject.SOParticipantConfig, error) {
	return l.h.GetParticipantConfig(ctx)
}

// GetTransformer returns the transformer used for the root state and operations.
func (l *lsoStateSnapshot) GetTransformer(ctx context.Context) (*block_transform.Transformer, error) {
	return l.h.GetTransformer(ctx)
}

// GetRootInner attempts to decode the current SORootInner and returns it.
func (l *lsoStateSnapshot) GetRootInner(ctx context.Context) (*sobject.SORootInner, error) {
	return l.h.GetRootInner(ctx)
}

// GetTransformInfo returns redacted transform configuration for display.
func (l *lsoStateSnapshot) GetTransformInfo(ctx context.Context) (*sobject.TransformInfo, error) {
	return l.h.GetTransformInfo(ctx)
}

// ProcessOperations processes operations as a validator calling cb.
func (l *lsoStateSnapshot) ProcessOperations(
	ctx context.Context,
	ops []*sobject.SOOperation,
	cb sobject.SnapshotProcessOpsFunc,
) (
	nextRoot *sobject.SORoot,
	rejectedOps []*sobject.SOOperationRejection,
	acceptedOps []*sobject.SOOperation,
	err error,
) {
	return l.h.ProcessOperations(ctx, ops, cb)
}

// _ is a type assertion
var _ sobject.SharedObjectStateSnapshot = ((*lsoStateSnapshot)(nil))
