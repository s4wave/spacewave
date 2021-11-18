package forge_target

import (
	"github.com/aperturerobotics/hydra/world"
)

// ivWorld is a input value for a world.
type ivWorld struct {
	// eng is the world engine
	eng world.Engine
	// ws is the world state
	ws world.WorldState
}

// NewInputValueWorld constructs a new InputValueWorld with a world handle.
// eng can be nil
func NewInputValueWorld(eng world.Engine, ws world.WorldState) InputValueWorld {
	return &ivWorld{eng: eng, ws: ws}
}

// GetInputType returns the input type of this value.
func (i *ivWorld) GetInputType() InputType {
	return InputType_InputType_WORLD
}

// Validate checks the input value.
func (i *ivWorld) Validate() error {
	// noop
	return nil
}

// IsEmpty checks if the value is "empty."
func (i *ivWorld) IsEmpty() bool {
	return i.ws == nil
}

// GetWorldEngine returns the world engine, if available.
// May return nil if unavailable.
func (i *ivWorld) GetWorldEngine() world.Engine {
	return i.eng
}

// GetWorldState returns the world state.
// Should not return nil.
func (i *ivWorld) GetWorldState() world.WorldState {
	return i.ws
}

// _ is a type assertion
var _ InputValueWorld = ((*ivWorld)(nil))
