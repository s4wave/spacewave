//go:build goscript

package function_run

import (
	"context"

	"github.com/aperturerobotics/fastjson"
	quickjswasi "github.com/aperturerobotics/go-quickjs-wasi-reactor"
	"github.com/pkg/errors"
	s4wave_function "github.com/s4wave/spacewave/sdk/function"
	function_run_browser "github.com/s4wave/spacewave/sdk/function/run/browser"
)

// Runner runs PureFunctions, each in a fresh QuickJS instance of the reactor
// the native runner embeds, on the browser's WebAssembly API.
type Runner struct{}

// NewRunner returns a runner.
func NewRunner() *Runner {
	return &Runner{}
}

// RunJSON runs fn on the protobuf JSON input and returns its output as JSON.
func (r *Runner) RunJSON(ctx context.Context, fn *s4wave_function.PureFunction, input []byte) ([]byte, error) {
	// Check the function and its input.
	if err := fn.Validate(); err != nil {
		return nil, err
	}
	if fastjson.ValidateBytes(input) != nil {
		return nil, errors.New("the input is not JSON")
	}

	// Run the function in the reactor.
	budget := fn.GetBudget()
	outcome, result, err := function_run_browser.Run(
		ctx,
		quickjswasi.QuickJSWASM,
		fn.GetModule(),
		budget.GetMemoryLimit(),
		budget.GetInterruptLimit(),
		string(input),
	)
	if err != nil {
		return nil, err
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}

	// Map how the run ended to the native runner's errors.
	switch outcome {
	case function_run_browser.OutcomeThrew:
		return nil, &FunctionError{Message: result}
	case function_run_browser.OutcomeInstructionBudget:
		return nil, ErrInstructionBudget
	case function_run_browser.OutcomeMemoryBudget:
		return nil, ErrMemoryBudget
	}
	return []byte(result), nil
}
