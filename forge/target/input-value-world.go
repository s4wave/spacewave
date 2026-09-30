package forge_target

import (
	"github.com/s4wave/spacewave/db/world"
)

// ivWorld retains a selected World capability and its registry scope.
type ivWorld struct {
	// engineID is the authoritative ID selected when resolving the World.
	engineID string
	// eng is the World engine, or nil for a nontransactional state.
	eng world.Engine
	// ws is the granted World state, or nil for an empty input.
	ws world.WorldState
}

// NewInputValueWorld constructs a new InputValueWorld with a world handle.
// engineID is selected by the granting component; eng may be nil.
func NewInputValueWorld(engineID string, eng world.Engine, ws world.WorldState) InputValueWorld {
	return &ivWorld{engineID: engineID, eng: eng, ws: ws}
}

// GetInputType returns the input type of this value.
func (i *ivWorld) GetInputType() InputType {
	return InputType_InputType_WORLD
}

// Validate checks the input value.
func (i *ivWorld) Validate() error {
	if i.eng != nil && i.engineID == "" {
		return world.ErrEmptyEngineID
	}
	return nil
}

// IsEmpty reports whether the value is empty.
func (i *ivWorld) IsEmpty() bool {
	return i.ws == nil
}

// GetWorldEngineID returns the selected World registry scope.
func (i *ivWorld) GetWorldEngineID() string {
	return i.engineID
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
var _ InputValueWorld = (*ivWorld)(nil)
