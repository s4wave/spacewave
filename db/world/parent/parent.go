package world_parent

import (
	"context"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/world"
)

// ParentPred is the parent predicate field.
var ParentPred = quad.IRI("parent")

// GetObjectParent returns the parent of a given object.
// Returns "" if the object has no parent.
func GetObjectParent(ctx context.Context, ws world.WorldState, key string) (string, error) {
	gq, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuad(
			world.KeyToGraphValue(key).String(),
			ParentPred.String(),
			"",
			"",
		), 1,
	)
	if err != nil || len(gq) == 0 {
		return "", err
	}
	return world.GraphValueToKey(gq[0].GetObj())
}

// BuildParentQuad returns a parent quad for a key -> parent object key.
func BuildParentQuad(objKey, parentKey string) quad.Quad {
	subjVal := world.KeyToGraphValue(objKey)
	parentVal := world.KeyToGraphValue(parentKey)
	return quad.Quad{
		Subject:   subjVal,
		Predicate: ParentPred,
		Object:    parentVal,
	}
}

// SetObjectParent sets the parent of a given object by writing a graph quad.
// If reset is set, deletes any non-matching <parent> quad in the same transaction.
// If parentKey is empty, clears the parent.
func SetObjectParent(ctx context.Context, ws world.WorldState, key, parentKey string, reset bool) error {
	if key == "" {
		return world.ErrEmptyObjectKey
	}
	// note: nextQuad.Object will be nil if parentKey is empty
	nextQuad := BuildParentQuad(key, parentKey)
	var delta []graph.Delta
	if err := ws.AccessCayleyGraph(ctx, true, func(ctx context.Context, h world.CayleyHandle) error {
		var exists bool
		var err error
		if reset {
			err = world.FilterIterateQuads(ctx, h, quad.Quad{
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
		}
		if !exists && nextQuad.Object != nil {
			delta = append(delta, graph.Delta{
				Quad:   nextQuad,
				Action: graph.Add,
			})
		}
		return err
	}); err != nil {
		return err
	}

	return world.ApplyGraphDeltas(ctx, ws, delta)
}

// ClearObjectParent removes all <parent> quads from an object.
func ClearObjectParent(ctx context.Context, ws world.WorldState, key string) error {
	if key == "" {
		return world.ErrEmptyObjectKey
	}
	lookupQuad := BuildParentQuad(key, "")
	var delta []graph.Delta
	if err := ws.AccessCayleyGraph(ctx, true, func(ctx context.Context, h world.CayleyHandle) error {
		err := world.FilterIterateQuads(ctx, h, lookupQuad, func(q quad.Quad) error {
			delta = append(delta, graph.Delta{
				Quad:   q,
				Action: graph.Delete,
			})
			return nil
		})
		if err != nil {
			return err
		}
		return err
	}); err != nil {
		return err
	}

	return world.ApplyGraphDeltas(ctx, ws, delta)
}
