package forge_target

import (
	"errors"

	forge_value "github.com/s4wave/spacewave/forge/value"
)

// NewOutputWithValue constructs a new Output with an in-line value.
func NewOutputWithValue(name string, val *forge_value.Value) *Output {
	return &Output{
		Name:       name,
		OutputType: OutputType_OutputType_VALUE,
		Value:      val,
	}
}

// IsNil checks if the object is nil.
func (o *Output) IsNil() bool {
	return o == nil
}

// Validate validates the Output object.
func (o *Output) Validate() error {
	if o.GetOutputType() == OutputType_OutputType_UNKNOWN {
		// assume empty
		return nil
	}
	if err := o.GetOutputType().Validate(false); err != nil {
		return err
	}
	switch o.GetOutputType() {
	case OutputType_OutputType_VALUE:
		if err := o.GetValue().Validate(true); err != nil {
			return err
		}
	case OutputType_OutputType_EXEC:
		if o.GetExecOutput() == "" {
			return errors.New("exec_output: name must be specified")
		}
	}
	return nil
}
