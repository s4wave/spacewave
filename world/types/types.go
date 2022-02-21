package world_types

import (
	"context"
	"strings"

	"github.com/aperturerobotics/hydra/world"
	"github.com/cayleygraph/cayley/graph"
	"github.com/cayleygraph/cayley/query/path"
	"github.com/cayleygraph/quad"
)

// TypesPrefix is the prefix string for all types identifiers.
const TypesPrefix = "types/"

// TypePred is the predicate linking a object to its type.
var TypePred quad.Value = quad.IRI("type")

// TypesState wraps a WorldState to implement type references.
// Objects have a <type> ref to a types/<type-id> object.
type TypesState struct {
	// ctx is the context
	ctx context.Context
	// world is the underlying world state handle.
	world world.WorldState
	// typePrefix is the key prefix used for types
	typePrefix string
}

// NewTypesState constructs a new TypesState interface.
func NewTypesState(ctx context.Context, w world.WorldState) *TypesState {
	return &TypesState{
		ctx:        ctx,
		world:      w,
		typePrefix: TypesPrefix,
	}
}

// GetObjectType returns the type of a given object.
// Returns "" if the object has no type.
func (p *TypesState) GetObjectType(key string) (string, error) {
	// AccessCayleyGraph calls a callback with a temporary Cayley graph handle.
	// All accesses of the handle should complete before returning cb.
	// Try to make access (queries) as short as possible.
	// Write operations will fail if the store is read-only.
	var typeKey string
	err := p.world.AccessCayleyGraph(false, func(h world.CayleyHandle) error {
		it := path.StartPath(h, world.KeyToGraphValue(key)).
			Out(TypePred).
			BuildIterator(p.ctx).
			Iterate()
		defer it.Close()
		// iterate until we find a suitable type key
		for it.Next(p.ctx) && typeKey == "" {
			res := it.Result()
			qv, err := h.NameOf(res)
			if err != nil {
				return err
			}
			key, err := world.QuadValueToKey(qv)
			if strings.HasPrefix(key, p.typePrefix) {
				typeKey = key
			}
		}
		return it.Err()
	})
	if err != nil || len(typeKey) == 0 {
		return "", err
	}
	// return world.GraphValueToKey(gq[0].GetObj())
	return typeKey[len(p.typePrefix):], nil
}

// BuildTypeQuadValue returns the quad value referring to the type.
func (p *TypesState) BuildTypeQuadValue(typeID string) quad.Value {
	if typeID == "" {
		return nil
	}
	return world.KeyToGraphValue(p.typePrefix + typeID)
}

// BuildTypeQuad returns a type quad for a key and type.
func (p *TypesState) BuildTypeQuad(objKey, typeID string) quad.Quad {
	subjVal := world.KeyToGraphValue(objKey)
	typeVal := p.BuildTypeQuadValue(typeID)
	return quad.Quad{
		Subject:   subjVal,
		Predicate: TypePred,
		Object:    typeVal,
	}
}

// SetObjectType sets the type of a given object by writing a graph quad.
func (p *TypesState) SetObjectType(key, typeID string) error {
	if key == "" || typeID == "" {
		return world.ErrEmptyObjectKey
	}
	nextQuad := p.BuildTypeQuad(key, typeID)
	return p.world.AccessCayleyGraph(true, func(h world.CayleyHandle) error {
		var exists bool
		var delta []graph.Delta
		err := world.FilterIterateQuads(p.ctx, h, quad.Quad{
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
func (p *TypesState) IterateObjectsWithType(
	typeID string,
	cb func(objKey string) (bool, error),
) error {
	if typeID == "" {
		return ErrTypeIDEmpty
	}
	if cb == nil {
		return nil
	}

	subCtx, subCtxCancel := context.WithCancel(p.ctx)
	defer subCtxCancel()
	return p.world.AccessCayleyGraph(false, func(h world.CayleyHandle) error {
		it := path.StartPath(h, p.BuildTypeQuadValue(typeID)).
			In(TypePred).
			BuildIterator(subCtx).Iterate()
		defer it.Close()
		for it.Next(subCtx) {
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
