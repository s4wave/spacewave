package electron

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
)

// unattachedRuntimeBus controls execution before a controller is attached.
type unattachedRuntimeBus struct {
	// Bus supplies methods unrelated to this controller lifetime test.
	bus.Bus

	// entered closes when execution reaches the unattached window.
	entered chan struct{}
	// complete ends execution without canceling its context.
	complete chan struct{}
	// removed receives the execution context error when removal finds no controller.
	removed chan error
	// runCtx is the context passed to ExecuteController.
	runCtx context.Context
}

// ExecuteController waits for cancellation or completion before an attachment exists.
func (b *unattachedRuntimeBus) ExecuteController(ctx context.Context, _ controller.Controller) error {
	b.runCtx = ctx
	close(b.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-b.complete:
		return nil
	}
}

// RemoveController records whether execution was canceled before removal.
func (b *unattachedRuntimeBus) RemoveController(controller.Controller) {
	b.removed <- b.runCtx.Err()
}

// TestControllerCancellationBeforeRuntimeAttachment verifies shutdown joins an
// execution that has not attached to the bus.
func TestControllerCancellationBeforeRuntimeAttachment(t *testing.T) {

	// Build an unattached runtime bus and controller under a cancelable context.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	b := &unattachedRuntimeBus{
		entered:  make(chan struct{}),
		complete: make(chan struct{}),
		removed:  make(chan error, 1),
	}
	r := &Controller{bus: b}
	e := &Electron{waitDone: make(chan struct{})}
	result := make(chan error, 1)
	go func() { result <- r.executeRuntimeController(ctx, e, r) }()

	// End the owning controller while runtime execution is still unattached.

	// Wait for runtime execution to start, then cancel the context.
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("runtime execution did not start")
	}
	cancel()

	// Removal alone cannot stop that execution; the derived context must end first.

	// Assert the runtime execution stopped with cancellation.
	select {
	case err := <-result:
		if err != context.Canceled {
			t.Fatalf("runtime exit = %v, want cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime execution did not stop after controller cancellation")
	}

	// Assert the runtime controller was removed with a canceled context.
	select {
	case err := <-b.removed:
		if err != context.Canceled {
			t.Fatalf("runtime context at removal = %v, want cancellation", err)
		}
	default:
		t.Fatal("runtime controller was not removed")
	}
}

// TestControllerRuntimeResultCancelsBeforeRemoval verifies that a completed
// execution is canceled before detachment while its parent context remains live.
func TestControllerRuntimeResultCancelsBeforeRemoval(t *testing.T) {

	// Build an unattached runtime bus and controller.
	b := &unattachedRuntimeBus{
		entered:  make(chan struct{}),
		complete: make(chan struct{}),
		removed:  make(chan error, 1),
	}
	r := &Controller{bus: b}
	e := &Electron{waitDone: make(chan struct{})}
	result := make(chan error, 1)
	go func() { result <- r.executeRuntimeController(t.Context(), e, r) }()

	// Finish execution while the owning context remains live.

	// Wait for runtime execution to start, then complete it.
	select {
	case <-b.entered:
	case <-time.After(time.Second):
		t.Fatal("runtime execution did not start")
	}
	close(b.complete)

	// The runtime context must be canceled before the controller is removed.

	// Assert the runtime context was canceled before removal.
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("runtime exit = %v, want nil", err)
		}
	case <-time.After(time.Second):
		t.Fatal("runtime execution did not return")
	}
	select {
	case err := <-b.removed:
		if err != context.Canceled {
			t.Fatalf("runtime context at removal = %v, want cancellation", err)
		}
	default:
		t.Fatal("runtime controller was not removed")
	}
}

// _ is a type assertion.
var _ bus.Bus = (*unattachedRuntimeBus)(nil)
