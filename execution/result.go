package forge_execution

import (
	forge_target "github.com/aperturerobotics/forge/target"
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/pkg/errors"
)

// FilterExecutionOutputs returns only valid outputs for completed executions.
// if allowFailed, skips any failed or invalid executions.
func FilterExecutionOutputs(execs []*Execution, allowFailed bool, minComplete int) ([]*Execution, [][]*forge_value.Value, error) {
	valid := make([]*Execution, 0, len(execs))
	validOutputs := make([][]*forge_value.Value, 0, cap(valid))
	for i, exec := range execs {
		if err := exec.Validate(); err != nil {
			if allowFailed {
				continue
			}
			return nil, nil, errors.Wrapf(err, "executions[%d]", i)
		}
		if exec.GetExecutionState() != State_ExecutionState_COMPLETE {
			continue
		}
		valid = append(valid, exec)
		validOutputs = append(validOutputs, exec.GetValueSet().GetOutputs())
	}
	if minComplete != 0 && len(valid) < minComplete {
		return nil, nil, errors.Errorf(
			"%d complete executions required: found %d",
			minComplete,
			len(valid),
		)
	}
	if len(valid) == 0 {
		return nil, nil, errors.New("no valid and complete executions")
	}
	return valid, validOutputs, nil
}

// ComputeExecutionOutputs computes the result from a set of Executions.
// if allowFailed is set, any failed or invalid executions are skipped.
// if no valid executions are in the list, returns an error.
// If minComplete != 0 and len(valid execs) < minComplete, fails.
func ComputeExecutionOutputs(
	outputs []*forge_target.Output,
	execOutputVals [][]*forge_value.Value,
	allowFailed bool,
) ([]*forge_value.Value, error) {
	var prevOutputs []*forge_value.Value
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
			return nil, errors.Wrapf(
				err, "execution outputs mismatch: len(%d) != len(%d)",
				len(prevOutputs), len(execOutpVals),
			)
		}
		if !forge_value.CompareValueSet(prevOutputs, execOutpVals) {
			return nil, errors.New("execution outputs mismatch: values are different")
		}
	}

	return prevOutputs, nil
}
