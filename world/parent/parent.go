package world_parent

import (
	"context"

	"github.com/aperturerobotics/hydra/world"
	world_cayley "github.com/aperturerobotics/hydra/world/cayley"
	"github.com/cayleygraph/cayley/graph"
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

// SetObjectParent sets the parent of a given object by writing a graph quad.
// Attempts to delete any non-matching <parent> quad in the same transaction.
// If parentKey is empty, clears the parent.
func (p *ParentState) SetObjectParent(ctx context.Context, key, parentKey string) error {
	if key == "" || parentKey == "" {
		return world.ErrEmptyObjectKey
	}
	nextQuad := p.BuildParentQuad(key, parentKey)
	return p.world.AccessCayleyGraph(true, func(h world.CayleyHandle) error {
		var exists bool
		var delta []graph.Delta
		err := world_cayley.FilterIterateQuads(ctx, h, quad.Quad{
			Subject:   nextQuad.Subject,
			Predicate: nextQuad.Predicate,
		}, func(q quad.Quad) error {
			if nextQuad.Object != nil && q.Object == nextQuad.Object {
				exists = true
			} else {
				delta = append(delta, graph.Delta{
					Quad:   q,
					Action: graph.Delete,
				})
			}
			return nil
		})
		if err != nil {
			return err
		}
		if !exists && nextQuad.Object != nil {
			delta = append(delta, graph.Delta{
				Quad:   nextQuad,
				Action: graph.Add,
			})
		}
		if len(delta) != 0 {
			err = h.ApplyDeltas(delta, graph.IgnoreOpts{
				IgnoreDup:     true,
				IgnoreMissing: true,
			})
		}
		return err
	})
}

// BuildParentQuad returns a parent quad for a key -> parent object key.
func (p *ParentState) BuildParentQuad(objKey, parentKey string) quad.Quad {
	subjVal := world.KeyToGraphValue(objKey)
	parentVal := world.KeyToGraphValue(parentKey)
	return quad.Quad{
		Subject:   subjVal,
		Predicate: p.parentPred,
		Object:    parentVal,
	}
}

// TODO: Given a Path (or Shape?), determine which Objects have no <parent>.
