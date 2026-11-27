package forge_execution

import (
	"github.com/pkg/errors"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// ComputeExecutionOutputs computes the outputs shared by a set of executions.
// Every execution's outputs must be valid and equal. Returns nil for an empty set.
func ComputeExecutionOutputs(
	outputs []*forge_target.Output,
	execOutputVals []forge_value.ValueSlice,
) ([]*forge_value.Value, error) {
	var prevOutputs forge_value.ValueSlice
	for i, execOutputs := range execOutputVals {
		execOutpVals, err := forge_target.ComputeOutputs(outputs, execOutputs)
		if err != nil {
			return nil, errors.Wrap(err, "invalid execution outputs")
		}
		if i == 0 {
			prevOutputs = execOutpVals
			continue
		}

		// compare with previous outputs
		if len(prevOutputs) != len(execOutpVals) {
			return nil, errors.Errorf(
				"execution outputs mismatch: len(%d) != len(%d)",
				len(prevOutputs), len(execOutpVals),
			)
		}
		if !prevOutputs.Equals(execOutpVals) {
			return nil, errors.New("execution outputs mismatch: values are different")
		}
	}

	return prevOutputs, nil
}
