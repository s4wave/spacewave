//go:build js

// Package function_run_browser runs a PureFunction module in a fresh QuickJS
// instance of the reactor on the browser's WebAssembly API.
//
// GoScript replaces this package with its TypeScript implementation in the
// repository's gs directory, which runs the module with run.ts in the parent
// package. These declarations give the runner the package's API to compile
// against; compiled by Go, Run reports errUnsupported.
package function_run_browser

import (
	"context"

	"github.com/pkg/errors"
)

// errUnsupported is the error of Run outside GoScript.
var errUnsupported = errors.New("the browser PureFunction runner runs only when compiled with GoScript")

// Outcome is how a run that reached the harness ended.
type Outcome int

const (
	// OutcomeOutput is a run whose result is the function's output.
	OutcomeOutput Outcome = iota
	// OutcomeThrew is a run whose result is what the function threw.
	OutcomeThrew
	// OutcomeInstructionBudget is a run that passed its interrupt checks.
	OutcomeInstructionBudget
	// OutcomeMemoryBudget is a run that passed its memory limit.
	OutcomeMemoryBudget
)

// Run runs module's default export on the protobuf JSON input in a fresh
// instance of the reactor wasm, under the memory limit in bytes and the
// interrupt check limit. The reactor is compiled once per slice. result is the
// output in protobuf JSON form or the thrown message, as outcome says; err is
// a failure of the runner itself.
func Run(
	ctx context.Context,
	reactor []byte,
	module string,
	memoryLimit, interruptLimit uint32,
	input string,
) (outcome Outcome, result string, err error) {
	return 0, "", errUnsupported
}
