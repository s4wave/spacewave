package world_parent

import (
	"context"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/world"
)

// ParentPred is the parent predicate field.
var ParentPred = quad.IRI("parent")

const parentGraphLookupLimit uint32 = 1_000_000

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

	nextQuad := world.NewGraphQuadWithKeys(key, ParentPred.String(), parentKey, "")
	exists := false
	if reset {
		quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, ParentPred.String(), "", ""), parentGraphLookupLimit)
		if err != nil {
			return err
		}
		for _, q := range quads {
			if parentKey != "" && q.GetObj() == nextQuad.GetObj() {
				exists = true
				continue
			}
			if err := ws.DeleteGraphQuad(ctx, q); err != nil {
				return err
			}
		}
	}
	if !exists && parentKey != "" {
		return ws.SetGraphQuad(ctx, nextQuad)
	}
	return nil
}

// ClearObjectParent removes all <parent> quads from an object.
func ClearObjectParent(ctx context.Context, ws world.WorldState, key string) error {
	if key == "" {
		return world.ErrEmptyObjectKey
	}
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(key, ParentPred.String(), "", ""), parentGraphLookupLimit)
	if err != nil {
		return err
	}
	for _, q := range quads {
		if err := ws.DeleteGraphQuad(ctx, q); err != nil {
			return err
		}
	}
	return nil
}
