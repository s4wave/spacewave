package forge_lib_docker

import (
	"context"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// noopExecHandle supplies a synthetic execution without performing storage writes.
type noopExecHandle struct{}

// GetExecutionUniqueId returns the synthetic Docker execution identity.
func (noopExecHandle) GetExecutionUniqueId() string {
	return "test-exec"
}

// GetExecutionObjectKey identifies the fake's one durable attempt.
func (noopExecHandle) GetExecutionObjectKey() string { return "exec/test" }

// GetExecutionClaimEpoch reports no claim for this standalone test handle.
func (noopExecHandle) GetExecutionClaimEpoch() uint64 { return 0 }

// GetPeerId supplies the unauthenticated test caller.
func (noopExecHandle) GetPeerId() peer.ID {
	return ""
}

// GetTimestamp supplies a fixed empty timestamp.
func (noopExecHandle) GetTimestamp() *timestamp.Timestamp {
	return &timestamp.Timestamp{}
}

// AccessStorage accepts the unused storage callback.
func (noopExecHandle) AccessStorage(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	return nil
}

// SetOutputs accepts outputs without retaining them.
func (noopExecHandle) SetOutputs(
	ctx context.Context,
	outps forge_value.ValueSlice,
	clearOld bool,
) error {
	return nil
}

// WriteLog accepts logs without retaining them.
func (noopExecHandle) WriteLog(ctx context.Context, level, message string) error {
	return nil
}

// SetWaitingPlugin accepts wait status without writing an Execution.
func (noopExecHandle) SetWaitingPlugin(context.Context, string) error { return nil }

// _ verifies the complete execution handle contract.
var _ forge_target.ExecControllerHandle = (*noopExecHandle)(nil)
