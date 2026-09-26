package plugin_space_runtime

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
)

// directiveRecorder records every directive it sees and resolves it idle.
type directiveRecorder struct {
	// mu guards dirs.
	mu sync.Mutex
	// dirs is every directive seen, in arrival order.
	dirs []directive.Directive
	// seen receives one signal per directive.
	seen chan struct{}
}

// newDirectiveRecorder constructs a directive recorder.
func newDirectiveRecorder() *directiveRecorder {
	return &directiveRecorder{seen: make(chan struct{}, 16)}
}

// GetControllerInfo returns information about the controller.
func (r *directiveRecorder) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/directive-recorder",
		controller.MustParseVersion("0.0.1"),
		"records directives",
	)
}

// Execute executes the controller.
func (r *directiveRecorder) Execute(context.Context) error {
	return nil
}

// HandleDirective records the directive.
func (r *directiveRecorder) HandleDirective(
	_ context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	r.mu.Lock()
	r.dirs = append(r.dirs, inst.GetDirective())
	r.mu.Unlock()
	r.seen <- struct{}{}

	return directive.R(directive.NewFuncResolver(func(_ context.Context, handler directive.ResolverHandler) error {
		handler.MarkIdle(true)
		return nil
	}), nil)
}

// Close releases any resources used by the controller.
func (r *directiveRecorder) Close() error {
	return nil
}

// waitFor waits for the next directive and checks that it is equivalent to
// want.
func (r *directiveRecorder) waitFor(t *testing.T, want directive.Directive) {
	t.Helper()
	select {
	case <-r.seen:
	case <-time.After(time.Second):
		t.Fatalf("parent did not receive %T", want)
	}

	r.mu.Lock()
	got := r.dirs[len(r.dirs)-1]
	r.mu.Unlock()
	equivalent, ok := want.(directive.DirectiveWithEquiv)
	if !ok || !equivalent.IsEquivalent(got) {
		t.Fatalf("parent directive = %T, want %T", got, want)
	}
}

// assertNoMore checks that no further directive arrives.
func (r *directiveRecorder) assertNoMore(t *testing.T) {
	t.Helper()
	select {
	case <-r.seen:
		t.Fatal("a generation-local directive reached the parent")
	case <-time.After(50 * time.Millisecond):
	}
}

// _ is a type assertion
var _ controller.Controller = (*directiveRecorder)(nil)
