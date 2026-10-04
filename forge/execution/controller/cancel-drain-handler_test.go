package execution_controller

import (
	"context"

	space_exec "github.com/s4wave/spacewave/core/forge/exec"
)

// cancelDrainHandler separates observing cancellation from draining work.
type cancelDrainHandler struct {
	// started signals the beginning of target execution.
	started chan struct{}
	// canceled signals that execution observed cancellation.
	canceled chan struct{}
	// drain releases the target's remaining work.
	drain chan struct{}
}

// Execute waits for cancellation and retains execution custody until drained.
func (h *cancelDrainHandler) Execute(ctx context.Context) error {
	// Announce target startup and wait for execution cancellation.
	close(h.started)
	<-ctx.Done()

	// Announce cancellation while retaining target custody until drain completes.
	close(h.canceled)
	<-h.drain
	return ctx.Err()
}

// _ is a type assertion.
var _ space_exec.Handler = (*cancelDrainHandler)(nil)
