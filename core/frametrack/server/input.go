//go:build !js

package frametrack_server

import (
	"context"

	"github.com/s4wave/spacewave/core/frametrack"
)

// Input is one admitted viewer's input sink.
type Input interface {
	// Send forwards an event within the viewer's control-stream lifetime.
	Send(ctx context.Context, input *frametrack.Input) error
	// Close releases the viewer's held inputs.
	Close() error
}

// ObserveInput binds a stateless event callback for each viewer.
func ObserveInput(observe func(*frametrack.Input)) func() Input {
	return func() Input { return inputObserver(observe) }
}

// inputObserver applies events to a source without tracking held inputs.
type inputObserver func(*frametrack.Input)

// Send applies an event to the source.
func (f inputObserver) Send(_ context.Context, input *frametrack.Input) error {
	f(input)
	return nil
}

// Close ends the binding.
func (f inputObserver) Close() error {
	return nil
}

// _ is a type assertion.
var _ Input = inputObserver(nil)
