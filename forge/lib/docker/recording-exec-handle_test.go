package forge_lib_docker

import "context"

// recordedLog is one retained Execution log entry.
type recordedLog struct {
	// level is the retained output severity.
	level string
	// message is the retained container output.
	message string
}

// recordingExecHandle records output written by the Docker controller.
type recordingExecHandle struct {
	// noopExecHandle supplies the synthetic execution contract.
	noopExecHandle
	// logs records output entries after execution completes.
	logs []recordedLog
}

// WriteLog records one container output stream.
func (h *recordingExecHandle) WriteLog(ctx context.Context, level, message string) error {
	h.logs = append(h.logs, recordedLog{level, message})
	return nil
}
