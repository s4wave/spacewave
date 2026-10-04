package execution_controller

import (
	"context"

	space_exec "github.com/s4wave/spacewave/core/forge/exec"
)

// finishHandler returns success once released.
type finishHandler struct {
	// started signals the beginning of target execution.
	started chan struct{}
	// finish releases the target's successful result.
	finish chan struct{}
}

// Execute signals start and returns success when released.
func (h *finishHandler) Execute(context.Context) error {
	close(h.started)
	<-h.finish
	return nil
}

// _ is a type assertion.
var _ space_exec.Handler = (*finishHandler)(nil)
