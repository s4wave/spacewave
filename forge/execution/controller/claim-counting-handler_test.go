package execution_controller

import (
	"context"
	"sync/atomic"

	space_exec "github.com/s4wave/spacewave/core/forge/exec"
)

// claimCountingHandler counts target starts across controller reconstruction.
type claimCountingHandler struct {
	// invocations retains the shared target invocation count.
	invocations *atomic.Int32
}

// Execute records one target invocation.
func (h *claimCountingHandler) Execute(context.Context) error {
	h.invocations.Add(1)
	return nil
}

// _ is a type assertion.
var _ space_exec.Handler = (*claimCountingHandler)(nil)
