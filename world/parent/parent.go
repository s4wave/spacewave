package world_parent

import "github.com/aperturerobotics/hydra/world"

// ParentState wraps a WorldState to implement garbage collection.
// Objects have a single <parent> edge to their parent object.
type ParentState struct {
	// WorldState is the underlying world state handle.
	world.WorldState
}

// Config is optional configuration for the ParentState.
type Config struct {
}

// NewParentState constructs a new ParentState interface.
func NewParentState(w world.WorldState, conf *Config) *ParentState {
	return &ParentState{
		WorldState: w,
	}
}

// GetObjectParent returns the parent of a given object.
// Returns nil if the object has no parent.
func (p *ParentState) GetObjectParent(key string) {
}
