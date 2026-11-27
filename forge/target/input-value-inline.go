package forge_target

import forge_value "github.com/s4wave/spacewave/forge/value"

// ivInline contains one resolved Value.
type ivInline struct {
	// v is the resolved value.
	v *forge_value.Value
	// inputType distinguishes literal values from Task outputs.
	inputType InputType
}

// NewInputValueInline constructs a new inputValueInline from a Value.
func NewInputValueInline(v *forge_value.Value) inputValueInline {
	return &ivInline{v: v, inputType: InputType_InputType_VALUE}
}

// newInputValueTaskOutput constructs a resolved Task output value.
func newInputValueTaskOutput(v *forge_value.Value) inputValueInline {
	return &ivInline{v: v, inputType: InputType_InputType_TASK_OUTPUT}
}

// GetInputType returns the input type of this value.
func (i *ivInline) GetInputType() InputType {
	return i.inputType
}

// Validate checks the input value.
func (i *ivInline) Validate() error {
	return i.v.Validate(true)
}

// IsEmpty reports whether the resolved value is empty.
func (i *ivInline) IsEmpty() bool {
	return i.v.IsEmpty()
}

// GetValue returns the value.
func (i *ivInline) GetValue() *forge_value.Value {
	return i.v
}

// _ is a type assertion
var _ inputValueInline = (*ivInline)(nil)
