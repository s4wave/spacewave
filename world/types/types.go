package world_types

import (
	"context"
	"strings"

	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/cayley"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/query/path"
	"github.com/cayleygraph/quad"
	"github.com/pkg/errors"
)

// TypesPrefix is the prefix string for all types identifiers.
const TypesPrefix = "types/"

// TypePred is the predicate linking a object to its type.
var TypePred quad.Value = quad.IRI("type")

// BuildTypeQuadValue returns the quad value referring to the type.
func BuildTypeQuadValue(typeID string) quad.Value {
	if typeID == "" {
		return nil
	}
	return world.KeyToGraphValue(TypesPrefix + typeID)
}

// BuildTypeQuad returns a type quad for a key and type.
func BuildTypeQuad(objKey, typeID string) quad.Quad {
	subjVal := world.KeyToGraphValue(objKey)
	typeVal := BuildTypeQuadValue(typeID)
	return quad.Quad{
		Subject:   subjVal,
		Predicate: TypePred,
		Object:    typeVal,
	}
}

// LimitNodesToTypes limits the matched nodes to the given types in the Path.
func LimitNodesToTypes(path *cayley.Path, typeIDs ...string) *cayley.Path {
	typeNodes := make([]quad.Value, len(typeIDs))
	for i, typeID := range typeIDs {
		typeNodes[i] = BuildTypeQuadValue(typeID)
	}
	return path.Has(TypePred, typeNodes...)
}

// GetObjectType returns the type of a given object.
// Returns "" if the object has no type.
func GetObjectType(ctx context.Context, ws world.WorldState, key string) (string, error) {
	// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
	// All accesses of the handle should complete before returning cb.
	// Try to make access (queries) as short as possible.
	// Write operations will fail if the store is read-only.
	var typeKey string
	err := ws.AccessCayleyGraph(ctx, false, func(h world.CayleyHandle) error {
		it := path.StartPath(h, world.KeyToGraphValue(key)).
			Out(TypePred).
			BuildIterator(ctx).
			Iterate()
		defer it.Close()
		// iterate until we find a suitable type key
		for it.Next(ctx) && typeKey == "" {
			res := it.Result()
			qv, err := h.NameOf(res)
			if err != nil {
				return err
			}
			key, err := world.QuadValueToKey(qv)
			if err != nil {
				return err
			}
			if strings.HasPrefix(key, TypesPrefix) {
				typeKey = key
			}
		}
		return it.Err()
	})
	if err != nil || len(typeKey) == 0 {
		return "", err
	}
	return typeKey[len(TypesPrefix):], nil
}

// CheckObjectType asserts that the object key exists and has the given type.
func CheckObjectType(ctx context.Context, ws world.WorldState, key, typeID string) error {
	objType, err := GetObjectType(ctx, ws, key)
	if err != nil {
		return err
	}
	if objType != typeID {
		if objType == "" {
			return errors.Errorf("object %s: expected object to exist w/ a valid type", key)
		}
		return errors.Errorf("object %s: expected type %s but got %q", key, typeID, objType)
	}
	return err
}

// SetObjectType sets the type of a given object by writing a graph quad.
func SetObjectType(ctx context.Context, ws world.WorldState, key, typeID string) error {
	if key == "" || typeID == "" {
		return world.ErrEmptyObjectKey
	}
	nextQuad := BuildTypeQuad(key, typeID)
	return ws.AccessCayleyGraph(ctx, true, func(h world.CayleyHandle) error {
		var exists bool
		var delta []graph.Delta
		err := world.FilterIterateQuads(ctx, h, quad.Quad{
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

// IterateObjectsWithType iterates over object keys with the given type ID.
func IterateObjectsWithType(
	rctx context.Context,
	ws world.WorldState,
	typeID string,
	cb func(objKey string) (bool, error),
) error {
	if typeID == "" {
		return ErrTypeIDEmpty
	}
	if cb == nil {
		return nil
	}

	ctx, subCtxCancel := context.WithCancel(rctx)
	defer subCtxCancel()
	return ws.AccessCayleyGraph(ctx, false, func(h world.CayleyHandle) error {
		it := path.StartPath(h, BuildTypeQuadValue(typeID)).
			In(TypePred).
			BuildIterator(ctx).
			Iterate()
		defer it.Close()
		for it.Next(ctx) {
			ref := it.Result()
			qv, err := h.NameOf(ref)
			if err != nil {
				return err
			}
			objKey, err := world.QuadValueToKey(qv)
			if err != nil {
				return err
			}
			ctnu, err := cb(objKey)
			if err != nil || !ctnu {
				return err
			}
		}
		return it.Err()
	})
}
