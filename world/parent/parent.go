package world_parent

import (
	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/quad"
)

// ParentState wraps a WorldState to implement garbage collection.
// Objects have a single <parent> edge to their parent object.
type ParentState struct {
	// world is the underlying world state handle.
	world world.WorldState
	// parentPred is the parent predicate field.
	parentPred quad.Value
}

// NewParentState constructs a new ParentState interface.
func NewParentState(w world.WorldState) *ParentState {
	return &ParentState{
		world:      w,
		parentPred: quad.IRI("parent"),
	}
}

// GetObjectParent returns the parent of a given object.
// Returns "" if the object has no parent.
func (p *ParentState) GetObjectParent(key string) (string, error) {
	gq, err := p.world.LookupGraphQuads(
		world.NewGraphQuad(
			world.KeyToGraphValue(key).String(),
			p.parentPred.String(),
			"",
			"",
		), 1,
	)
	if err != nil || len(gq) == 0 {
		return "", err
	}
	return world.GraphValueToKey(gq[0].GetObject())
}
