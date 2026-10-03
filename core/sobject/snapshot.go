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
	// GetConfigByHash returns the config with the given config chain hash.
	// Returns ErrConfigHistoryUnavailable when the config is not retained.
	GetConfigByHash(ctx context.Context, hash []byte) (*SharedObjectConfig, error)

	// GetTransformer returns the transformer of the current key epoch, which
	// encrypts new operations and checkpoints.
	GetTransformer(ctx context.Context) (*block_transform.Transformer, error)

	// GetTransformInfo returns redacted transform configuration for display.
	// Decrypts the local participant's grant to extract step configs, then
	// strips sensitive fields (encryption keys). Returns epoch and grant count.
	GetTransformInfo(ctx context.Context) (*TransformInfo, error)

	// GetCheckpoint returns the checkpoint body with its state data decoded.
	// Returns nil, nil before the first checkpoint.
	GetCheckpoint(ctx context.Context) (*SOCheckpointInner, error)

	// GetOperationSet returns the verified operations above the checkpoint.
	// Callers must not modify the returned set.
	GetOperationSet(ctx context.Context) (*SOOperationSet, error)

	// DecodeOperation returns the operation data decoded with the key of the
	// epoch the operation names. Returns ErrKeyEpochUnavailable when the local
	// peer holds no grant for that epoch.
	DecodeOperation(ctx context.Context, inner *SOOperationInner) ([]byte, error)
}
