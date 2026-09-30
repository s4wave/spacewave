package space_exec

import (
	"context"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
)

// pluginExecHandleStub retains logs and outputs in an isolated storage cursor.
type pluginExecHandleStub struct {
	// logs retains appended entries, read after logCh or call completion.
	logs []*PluginExecLog
	// logCh delivers entries after they have been retained.
	logCh chan *PluginExecLog
	// outputs retains copied values after response application.
	outputs forge_value.ValueSlice
	// cursor is borrowed for the lifetime of the test.
	cursor *bucket_lookup.Cursor
}

// SetWaitingPlugin accepts wait status without writing an Execution.
func (h *pluginExecHandleStub) SetWaitingPlugin(ctx context.Context, pluginID string) error {
	return nil
}

// GetExecutionUniqueId returns the synthetic execution identity.
func (h *pluginExecHandleStub) GetExecutionUniqueId() string {
	return "test-exec"
}

// GetExecutionObjectKey identifies the synthetic plugin execution attempt.
func (h *pluginExecHandleStub) GetExecutionObjectKey() string { return "exec/test" }

// GetExecutionClaimEpoch reports no claim for this standalone test handle.
func (h *pluginExecHandleStub) GetExecutionClaimEpoch() uint64 { return 0 }

// GetPeerId supplies the unauthenticated test caller.
func (h *pluginExecHandleStub) GetPeerId() peer.ID {
	return ""
}

// GetTimestamp supplies a fixed empty timestamp.
func (h *pluginExecHandleStub) GetTimestamp() *timestamp.Timestamp {
	return &timestamp.Timestamp{}
}

// AccessStorage borrows the configured cursor and releases any reference clone.
func (h *pluginExecHandleStub) AccessStorage(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	if h.cursor == nil {
		return nil
	}
	if ref != nil && !ref.GetRootRef().GetEmpty() {
		cs := h.cursor.Clone()
		defer cs.Release()
		cs.SetRootRef(ref.GetRootRef())
		return cb(cs)
	}
	return cb(h.cursor)
}

// SetOutputs retains a copy of the plugin outputs.
func (h *pluginExecHandleStub) SetOutputs(
	ctx context.Context,
	outputs forge_value.ValueSlice,
	clearOld bool,
) error {
	h.outputs = outputs.Clone()
	return nil
}

// WriteLog retains the log and signals subscribers after appending it.
func (h *pluginExecHandleStub) WriteLog(ctx context.Context, level, message string) error {
	log := &PluginExecLog{Level: level, Message: message}
	h.logs = append(h.logs, log)
	if h.logCh != nil {
		h.logCh <- log
	}
	return nil
}

// _ verifies the complete execution handle contract.
var _ forge_target.ExecControllerHandle = (*pluginExecHandleStub)(nil)
