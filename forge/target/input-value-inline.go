package forge_target

import forge_value "github.com/s4wave/spacewave/forge/value"

// ivInline is a input value containing a single Value.
type ivInline struct {
	v *forge_value.Value
}

// NewInputValueInline constructs a new InputValueInline from a Value.
func NewInputValueInline(v *forge_value.Value) InputValueInline {
	return &ivInline{v: v}
}

// GetInputType returns the input type of this value.
func (i *ivInline) GetInputType() InputType {
	return InputType_InputType_VALUE
}

// Validate checks the input value.
func (i *ivInline) Validate() error {
	return i.v.Validate(true)
}

// IsEmpty checks if the value is "empty."
func (i *ivInline) IsEmpty() bool {
	return i.v.IsEmpty()
}

// GetValue returns the value.
func (i *ivInline) GetValue() *forge_value.Value {
	return i.v
}

// _ is a type assertion
var _ InputValueInline = ((*ivInline)(nil))
