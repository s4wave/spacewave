package execution_controller

import (
	"context"
	"sync/atomic"

	"github.com/pkg/errors"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	forge_target "github.com/s4wave/spacewave/forge/target"
)

// interruptionHandler reports its first outcome, then waits before succeeding.
type interruptionHandler struct {
	// firstErr is the first invocation's transport interruption or handler failure.
	firstErr error
	// onFirst runs before the first outcome, allowing durable cancellation to race it.
	onFirst func(context.Context) error
	// runs counts invocations across reconstructed target controllers.
	runs *atomic.Int32
	// handle carries the claim granted to this invocation.
	handle forge_target.ExecControllerHandle
	// resumed receives the claim epoch of the second invocation.
	resumed chan uint64
	// finish releases the second invocation's successful result.
	finish <-chan struct{}
}

// Execute interrupts the first invocation without applying side effects.
func (h *interruptionHandler) Execute(ctx context.Context) error {
	// Preserve the typed first outcome through the plugin bridge's wrapping.
	if h.runs.Add(1) == 1 {
		if h.onFirst != nil {
			if err := h.onFirst(ctx); err != nil {
				return err
			}
		}
		return errors.Wrap(h.firstErr, "receive plugin execution stream")
	}

	// Report the retry's granted epoch and retain its result until released.
	h.resumed <- h.handle.GetExecutionClaimEpoch()
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-h.finish:
		return nil
	}
}

// _ is a type assertion.
var _ space_exec.Handler = (*interruptionHandler)(nil)
