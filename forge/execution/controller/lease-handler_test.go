package execution_controller_test

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
)

// leaseHandler retains the first target until cancellation and prevents recovery
// from succeeding if the former target has not drained.
type leaseHandler struct {
	// invocations counts target starts across both Workers.
	invocations *atomic.Int32
	// started signals the first target start.
	started chan struct{}
	// drained signals the first target's cancellation and drain.
	drained chan struct{}
}

// Execute blocks the first target and verifies its drain before running again.
func (h *leaseHandler) Execute(ctx context.Context) error {
	// Run the original target until the Worker's lifecycle is canceled.
	if h.invocations.Add(1) == 1 {
		close(h.started)
		<-ctx.Done()
		close(h.drained)
		return ctx.Err()
	}

	// Reject overlapping target execution during claim recovery.
	select {
	case <-h.drained:
		return nil
	default:
		return errors.New("reclaimer overlapped the original target")
	}
}
