package sobject

import (
	"context"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
)

// TransformInfo contains redacted transform configuration for display.
type TransformInfo struct {
	// Steps contains the transform steps with sensitive fields redacted.
	Steps []*block_transform.StepConfig
	// GrantCount is the number of participants with active grants.
	GrantCount uint32
}

// SharedObjectStateSnapshot is the state snapshot interface for the SharedObject.
type SharedObjectStateSnapshot interface {
	// GetParticipantConfigForPeer returns a participant in the accepted config.
	GetParticipantConfigForPeer(ctx context.Context, peerID string) (*SOParticipantConfig, error)
	// GetParticipantConfig returns the participant record for our participant.
	// uses the peer identity from the SharedObject.
	// returns ErrNotParticipant if the local peer is not a participant.
	GetParticipantConfig(ctx context.Context) (*SOParticipantConfig, error)
	// GetConfig returns the accepted config.
	GetConfig(ctx context.Context) (*SharedObjectConfig, error)
	// GetConfigByHash returns the config with the given config chain hash.
	// Returns ErrConfigHistoryUnavailable when the config is not retained.
	GetConfigByHash(ctx context.Context, hash []byte) (*SharedObjectConfig, error)
	// GetOwnership returns the ownership of the accepted config.
	GetOwnership(ctx context.Context) (*SOOwnership, error)

	// GetTransformer returns the transformer of the current key epoch, which
	// encrypts new operations and checkpoints.
	GetTransformer(ctx context.Context) (*block_transform.Transformer, error)

	// GetTransformInfo returns redacted transform configuration for display.
	// Decrypts the local participant's grant to extract step configs, then
	// strips sensitive fields (encryption keys). Returns epoch and grant count.
	GetTransformInfo(ctx context.Context) (*TransformInfo, error)

	// GetCheckpoint returns the checkpoint body with its state data decoded.
	// Returns nil, nil before the first checkpoint. Callers must not modify
	// the returned body.
	GetCheckpoint(ctx context.Context) (*SOCheckpointInner, error)

	// GetOperationSet returns the verified operations above the checkpoint.
	// Callers must not modify the returned set.
	GetOperationSet(ctx context.Context) (*SOOperationSet, error)

	// DecodeOperation returns the operation data decoded with the key of the
	// epoch the operation names. Returns ErrKeyEpochUnavailable when the local
	// peer holds no grant for that epoch.
	DecodeOperation(ctx context.Context, inner *SOOperationInner) ([]byte, error)
}

// watchStates calls fn with each state of so and its config as they change,
// as the local peer peerID. It returns when ctx ends or fn fails.
func watchStates(ctx context.Context, so SharedObject, fn func(snap SharedObjectStateSnapshot, cfg *SharedObjectConfig, peerID string) error) error {
	// Watch the state as the local peer.
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return err
	}
	defer rel()
	peerID := so.GetPeerID().String()

	// Call fn with each state and its config.
	var snap SharedObjectStateSnapshot
	for {
		snap, err = ctr.WaitValueChange(ctx, snap, nil)
		if err != nil {
			return err
		}
		if snap == nil {
			continue
		}
		cfg, err := snap.GetConfig(ctx)
		if err != nil {
			return err
		}
		if err := fn(snap, cfg, peerID); err != nil {
			return err
		}
	}
}
