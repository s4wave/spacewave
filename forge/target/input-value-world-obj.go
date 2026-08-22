package forge_target

import (
	"errors"

	"github.com/s4wave/spacewave/db/world"
)

// ivWorldObject is a input value for a world object.
type ivWorldObject struct {
	InputValueInline
	InputValueWorld
	objs world.ObjectState
	err  error
}

// NewInputValueWorldObject constructs a new InputValueWorldObject from its
// component values. Nil inline and world components are replaced with empty
// ones. objs carries the resolved object state and err any resolution error;
// Validate reports err until it is cleared.
func NewInputValueWorldObject(
	inline InputValueInline,
	wrld InputValueWorld,
	objs world.ObjectState,
	err error,
) InputValueWorldObject {
	// ensure no nil references on interfaces
	if inline == nil {
		inline = NewInputValueInline(nil)
	}
	if wrld == nil {
		wrld = NewInputValueWorld(nil, nil)
	}
	return &ivWorldObject{InputValueInline: inline, InputValueWorld: wrld, objs: objs, err: err}
}

// GetInputType returns the input type of this value.
func (i *ivWorldObject) GetInputType() InputType {
	return InputType_InputType_WORLD_OBJECT
}

// Validate checks the input value.
func (i *ivWorldObject) Validate() error {
	if i.err != nil {
		return i.err
	}
	if i.InputValueInline != nil && !i.InputValueInline.IsEmpty() {
		if err := i.InputValueInline.Validate(); err != nil {
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

// IsEmpty checks if the value is "empty."
func (i *ivWorldObject) IsEmpty() bool {
	return i.objs == nil
}

// GetWorldObject returns the world object state handle.
func (i *ivWorldObject) GetWorldObject() world.ObjectState {
	return i.objs
}

// _ is a type assertion
var _ InputValueWorldObject = (*ivWorldObject)(nil)
