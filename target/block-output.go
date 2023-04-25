package forge_target

import (
	"errors"

	forge_value "github.com/aperturerobotics/forge/value"
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
func (i *Output) Validate() error {
	if i.GetOutputType() == OutputType_OutputType_UNKNOWN {
		// assume empty
		return nil
	}
	if err := i.GetOutputType().Validate(false); err != nil {
		return err
	}
	switch i.GetOutputType() {
	case OutputType_OutputType_VALUE:
		if err := i.GetValue().Validate(true); err != nil {
			return err
		}
	case OutputType_OutputType_EXEC:
		if i.GetExecOutput() == "" {
			return errors.New("exec_output: name must be specified")
		}
	}
	return nil
}
