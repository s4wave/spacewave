// Package function_run runs PureFunctions in the QuickJS-NG WASI reactor:
// under wazero when compiled by Go, and on the browser's WebAssembly API when
// compiled with GoScript. Both run the same reactor build and harness, so the
// same input gives the same output.
package function_run

import (
	"context"

	"github.com/pkg/errors"
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
)

// ErrInstructionBudget is returned when a run passes its interrupt checks.
var ErrInstructionBudget = errors.New("the function ran past its instruction budget")

// ErrMemoryBudget is returned when a run passes its memory limit.
var ErrMemoryBudget = errors.New("the function ran past its memory budget")

// FunctionError is an error the function threw or returned.
type FunctionError struct {
	// Message is the error's string form.
	Message string
}

// Error returns the error message.
func (e *FunctionError) Error() string {
	return e.Message
}

// JSONMarshaler is a message with a protobuf JSON form, such as a generated
// protobuf message.
type JSONMarshaler interface {
	// MarshalJSON returns the message's protobuf JSON form.
	MarshalJSON() ([]byte, error)
}

// JSONUnmarshaler decodes a message from its protobuf JSON form.
type JSONUnmarshaler interface {
	// UnmarshalJSON decodes the message from its protobuf JSON form.
	UnmarshalJSON([]byte) error
}

// Run runs fn on in and stores its result in out.
func (r *Runner) Run(ctx context.Context, fn *s4wave_function.PureFunction, in JSONMarshaler, out JSONUnmarshaler) error {
	// Run the function on the input's protobuf JSON form.
	input, err := in.MarshalJSON()
	if err != nil {
		return errors.Wrap(err, "encode input")
	}
	output, err := r.RunJSON(ctx, fn, input)
	if err != nil {
		return err
	}

	// Decode the function's output.
	if err := out.UnmarshalJSON(output); err != nil {
		return errors.Wrap(err, "decode output")
	}
	return nil
}
