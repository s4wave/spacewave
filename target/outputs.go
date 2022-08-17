package forge_target

import (
	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/pkg/errors"
)

// ComputeOutput computes the output value for a Execution output value.
//
// returns an empty value (Type=0) for any unset outputs.
func ComputeOutput(output *Output, execValues []*forge_value.Value) (*forge_value.Value, error) {
	outpType := output.GetOutputType()
	var outpVal *forge_value.Value
	switch outpType {
	case OutputType_OutputType_VALUE:
		outpVal = output.GetValue().Clone()
	case OutputType_OutputType_EXEC:
		execOutp := output.GetExecOutput()
		for _, execVal := range execValues {
			if execVal.GetName() == execOutp {
				outpVal = execVal.Clone()
				break
			}
		}
	default:
		return nil, errors.Wrap(ErrUnknownOutputType, outpType.String())
	}

	if outpVal == nil {
		outpVal = &forge_value.Value{Name: output.GetName()}
	} else {
		outpVal.Name = output.GetName()
		if err := outpVal.Validate(true); err != nil {
			return nil, err
		}
	}

	return outpVal, nil
}

// ComputeOutputs computes the output set for a list of Execution output values.
func ComputeOutputs(outputs []*Output, execValues []*forge_value.Value) (forge_value.ValueSlice, error) {
	var err error
	outpVals := make(forge_value.ValueSlice, len(outputs))
	for i, outp := range outputs {
		outpVals[i], err = ComputeOutput(outp, execValues)
		if err != nil {
			return nil, errors.Wrap(err, outp.GetName())
		}
	}
	outpVals.SortByName()
	return outpVals, nil
}
