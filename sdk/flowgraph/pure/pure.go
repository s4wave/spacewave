// Package flowgraph_pure defines the pure functions that pure Steps of a
// Flowgraph run evaluate, and the directive that finds them on the bus.
package flowgraph_pure

import (
	"context"

	"github.com/pkg/errors"
)

// MaxValueSize bounds an output or state a pure function returns, in bytes.
// A run holds its pending arrivals in its own block, so an unbounded value
// would grow every run write.
const MaxValueSize = 64 << 10

// Function evaluates the pure functions a component registers. Evaluate reads
// only its request and never writes the World. It returns an error only when
// the evaluation could not run, such as a lost connection to the component; a
// function that cannot conclude on its inputs returns a PureFault, which
// pauses the run. Both SRPCPureFunctionServiceServer and
// SRPCPureFunctionServiceClient implement it.
type Function interface {
	// Evaluate computes a pure Step's result from its inputs and state.
	Evaluate(ctx context.Context, req *EvaluateRequest) (*EvaluateResponse, error)
}

// Validate requires exactly one outcome and bounds a concluded result.
func (r *EvaluateResponse) Validate() error {
	switch result := r.GetResult().(type) {
	case *EvaluateResponse_Concluded:
		if result.Concluded.GetPort() == "" {
			return errors.New("pure function concluded without a port")
		}
		if len(result.Concluded.GetOutput()) > MaxValueSize {
			return errors.Errorf("pure function output is %d bytes, over %d", len(result.Concluded.GetOutput()), MaxValueSize)
		}
		if len(result.Concluded.GetNextState()) > MaxValueSize {
			return errors.Errorf("pure function state is %d bytes, over %d", len(result.Concluded.GetNextState()), MaxValueSize)
		}
		return nil
	case *EvaluateResponse_Fault:
		return nil
	default:
		return errors.New("pure function returned no result")
	}
}
