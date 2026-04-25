package sobject

import (
	"context"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
)

// SnapshotProcessOpsFunc is a function which processes operations against a state.
// cb is called with the state snapshot and the decoded inner state.
// If rawNextStateData is nil, no changes will be applied to the state (no-op).
type SnapshotProcessOpsFunc = func(
	ctx context.Context,
	currentStateData []byte,
	ops []*SOOperationInner,
) (rawNextStateData *[]byte, opResults []*SOOperationResult, err error)

// TransformInfo contains redacted transform configuration for display.
type TransformInfo struct {
	// Steps contains the transform steps with sensitive fields redacted.
	Steps []*block_transform.StepConfig
	// GrantCount is the number of participants with active grants.
	GrantCount uint32
}

// SharedObjectStateSnapshot is the state snapshot interface for the SharedObject.
type SharedObjectStateSnapshot interface {
	// GetParticipantConfig returns the participant record for our participant.
	// uses the peer identity from the SharedObject.
	// returns ErrNotParticipant if the local peer is not a participant.
	GetParticipantConfig(ctx context.Context) (*SOParticipantConfig, error)

	// GetTransformer returns the transformer used for the root state and operations.
	// Returns the same transformer used for encoding/decoding the root state.
	GetTransformer(ctx context.Context) (*block_transform.Transformer, error)

	// GetTransformInfo returns redacted transform configuration for display.
	// Decrypts the local participant's grant to extract step configs, then
	// strips sensitive fields (encryption keys). Returns epoch and grant count.
	GetTransformInfo(ctx context.Context) (*TransformInfo, error)

	// GetOpQueue returns the operation queue for our participant.
	// Returns the list of queued ops with nonces + the local queue (no nonce yet).
	// uses the peer identity from the SharedObject.
	GetOpQueue(ctx context.Context) ([]*SOOperation, []*QueuedSOOperation, error)

	// GetRootInner attempts to decode the current SORootInner and returns it.
	// uses the peer identity from the SharedObject to decode.
	//
	// If the shared object is blank, returns nil, nil.
	GetRootInner(ctx context.Context) (*SORootInner, error)

	// ProcessOperations processes operations as a validator calling cb.
	// The ops should be processed in the order they are provided.
	// The results must be a subset of ops (but does not need to have all ops).
	// Returns the updated root state.
	// This function is called by the SharedObject controller.
	// You probably want to call SharedObject.ProcessOperations instead.
	ProcessOperations(
		ctx context.Context,
		ops []*SOOperation,
		cb SnapshotProcessOpsFunc,
	) (
		nextRoot *SORoot,
		rejectedOps []*SOOperationRejection,
		acceptedOps []*SOOperation,
		err error,
	)
}
