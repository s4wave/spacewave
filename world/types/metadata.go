package world_types

import (
	"context"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/shape"
	"github.com/aperturerobotics/hydra/world"
	world_parent "github.com/aperturerobotics/hydra/world/parent"
)

// ObjectMetadata holds the type and parent metadata for a single object.
type ObjectMetadata struct {
	// ObjectKey is the object key.
	ObjectKey string
	// TypeID is the type of the object, empty if none.
	TypeID string
	// ParentObjectKey is the parent object key, empty if none.
	ParentObjectKey string
}

// GetObjectMetadataBatch returns the type and parent metadata for a list of
// object keys using indexed graph passes within a single Cayley transaction
// rather than 2N individual lookups.
//
// The result slice preserves the input key order.
func GetObjectMetadataBatch(ctx context.Context, ws world.WorldState, keys []string) ([]*ObjectMetadata, error) {
	if len(keys) == 0 {
		return nil, nil
	}

	result := make([]*ObjectMetadata, len(keys))
	subjects := make([]quad.Value, len(keys))
	resultByKey := make(map[string][]*ObjectMetadata, len(keys))
	for i, key := range keys {
		md := &ObjectMetadata{ObjectKey: key}
		result[i] = md
		subjects[i] = world.KeyToGraphValue(key)
		resultByKey[key] = append(resultByKey[key], md)
	}

	err := ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		// Type lookup: batched subject+predicate pass uses the (subject,predicate) index.
		if err := iterateSubjectPredicateQuads(ctx, h, subjects, TypePred, func(q quad.Quad) error {
			return setTypeBatch(q, resultByKey)
		}); err != nil {
			return err
		}

		// Parent lookup: batched subject+predicate pass uses the same index.
		return iterateSubjectPredicateQuads(ctx, h, subjects, world_parent.ParentPred, func(q quad.Quad) error {
			return setParentBatch(q, resultByKey)
		})
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// iterateSubjectPredicateQuads iterates quads matching any of the given subjects and one predicate.
func iterateSubjectPredicateQuads(
	ctx context.Context,
	h world.CayleyHandle,
	subjects []quad.Value,
	predicate quad.Value,
	cb func(q quad.Quad) error,
) error {
	if len(subjects) == 0 {
		return nil
	}

	return world.OptimizeIterateQuads(ctx, h, shape.Quads{
		{Dir: quad.Subject, Values: shape.Lookup(subjects)},
		{Dir: quad.Predicate, Values: shape.Lookup([]quad.Value{predicate})},
	}, cb)
}

// setTypeBatch updates result metadata from a type quad.
func setTypeBatch(q quad.Quad, resultByKey map[string][]*ObjectMetadata) error {
	if q.Subject == nil || q.Object == nil {
		return nil
	}

	objKey, err := world.QuadValueToKey(q.Subject)
	if err != nil {
		return err
	}
	typeKey, err := world.QuadValueToKey(q.Object)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(typeKey, TypesPrefix) {
		return nil
	}
	typeID := typeKey[len(TypesPrefix):]
	for _, md := range resultByKey[objKey] {
		if md.TypeID == "" {
			md.TypeID = typeID
		}
	}
	return nil
}

// setParentBatch updates result metadata from a parent quad.
func setParentBatch(q quad.Quad, resultByKey map[string][]*ObjectMetadata) error {
	if q.Subject == nil || q.Object == nil {
		return nil
	}

	objKey, err := world.QuadValueToKey(q.Subject)
	if err != nil {
		return err
	}
	parentKey, err := world.QuadValueToKey(q.Object)
	if err != nil {
		return err
	}
	for _, md := range resultByKey[objKey] {
		md.ParentObjectKey = parentKey
	}
	return nil
}
