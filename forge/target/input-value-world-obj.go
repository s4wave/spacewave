package forge_target

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// ivWorldObject is a input value for a world object.
type ivWorldObject struct {
	// inputValueInline retains the resolved inline object value.
	inputValueInline
	// InputValueWorld retains the object World and its selected engine identity.
	InputValueWorld
	// objs retains the resolved object state.
	objs world.ObjectState
	// err records the input or typed factory resolution error.
	err error
}

// newInputValueWorldObject constructs a new inputValueWorldObject from its
// component values. Nil inline and world components are replaced with empty
// ones. objs carries the resolved object state and err carries any resolution
// error. Validate reports err until it is cleared.
func newInputValueWorldObject(
	inline inputValueInline,
	wrld InputValueWorld,
	objs world.ObjectState,
	err error,
) inputValueWorldObject {
	// Replace missing components with honest empty input values.
	if inline == nil {
		inline = NewInputValueInline(nil)
	}
	if wrld == nil {
		wrld = NewInputValueWorld("", nil, nil)
	}
	return &ivWorldObject{inputValueInline: inline, InputValueWorld: wrld, objs: objs, err: err}
}

// GetInputType returns the input type of this value.
func (i *ivWorldObject) GetInputType() InputType {
	return InputType_InputType_WORLD_OBJECT
}

// Validate checks the input value.
func (i *ivWorldObject) Validate() error {
	// Report object resolution failures before validating the input components.
	if i.err != nil {
		return i.err
	}
	if i.inputValueInline != nil && !i.inputValueInline.IsEmpty() {
		if err := i.inputValueInline.Validate(); err != nil {
			return err
		}
	}
	if i.InputValueWorld == nil || i.InputValueWorld.IsEmpty() {
		return errors.New("empty world input value")
	}
	if err := i.InputValueWorld.Validate(); err != nil {
		return err
	}
	return nil
}

// IsEmpty reports whether the value is empty.
func (i *ivWorldObject) IsEmpty() bool {
	return i.objs == nil
}

// _ is a type assertion
var _ inputValueWorldObject = (*ivWorldObject)(nil)
