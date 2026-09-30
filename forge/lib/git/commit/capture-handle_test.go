package forge_lib_git_commit

import (
	"context"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// captureHandle retains the commit controller's outputs in a borrowed World.
type captureHandle struct {
	// peerID is the authenticated test caller.
	peerID peer.ID
	// ts is the immutable test execution timestamp.
	ts *timestamp.Timestamp
	// accessFunc borrows the World storage scope.
	accessFunc world.AccessWorldStateFunc
	// outputs retains copies supplied by the controller.
	outputs forge_value.ValueSlice
}

// GetExecutionUniqueId returns the synthetic commit execution identity.
func (h *captureHandle) GetExecutionUniqueId() string {
	return "test-git-commit"
}

// GetExecutionObjectKey identifies the synthetic test attempt.
func (h *captureHandle) GetExecutionObjectKey() string { return "exec/test-git-commit" }

// GetExecutionClaimEpoch reports no claim for this standalone test handle.
func (h *captureHandle) GetExecutionClaimEpoch() uint64 { return 0 }

// GetPeerId returns the peer authenticated by the in-memory World.
func (h *captureHandle) GetPeerId() peer.ID {
	return h.peerID
}

// GetTimestamp returns the timestamp supplied by the test.
func (h *captureHandle) GetTimestamp() *timestamp.Timestamp {
	return h.ts
}

// AccessStorage forwards the callback within the borrowed World scope.
func (h *captureHandle) AccessStorage(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return h.accessFunc(ctx, ref, cb)
}

// SetOutputs retains output copies and clears prior values when requested.
func (h *captureHandle) SetOutputs(ctx context.Context, outps forge_value.ValueSlice, clearOld bool) error {
	if clearOld {
		h.outputs = nil
	}
	h.outputs = append(h.outputs, outps.Clone()...)
	return nil
}

// WriteLog accepts logs without retaining them.
func (h *captureHandle) WriteLog(ctx context.Context, level, message string) error {
	return nil
}

// SetWaitingPlugin accepts wait status without writing an Execution.
func (h *captureHandle) SetWaitingPlugin(context.Context, string) error { return nil }

// _ verifies the complete execution handle contract.
var _ forge_target.ExecControllerHandle = (*captureHandle)(nil)
