package world_types

import (
	"context"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
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
	subjects := make([]quad.Value, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	resultByKey := make(map[string][]*ObjectMetadata, len(keys))
	for i, key := range keys {
		md := &ObjectMetadata{ObjectKey: key}
		result[i] = md
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			subjects = append(subjects, world.KeyToGraphValue(key))
		}
		resultByKey[key] = append(resultByKey[key], md)
	}

	err := ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		// Type lookup uses concrete subject+predicate indexed passes to preserve full quads.
		if err := iterateSubjectPredicateQuads(ctx, h, subjects, TypePred, func(q quad.Quad) error {
			return setTypeBatch(q, resultByKey)
		}); err != nil {
			return err
		}

		// Parent lookup uses the same concrete indexed pass.
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

	for _, subject := range subjects {
		if err := world.IterateFilteredFullQuads(ctx, h, quad.Quad{
			Subject:   subject,
			Predicate: predicate,
		}, cb); err != nil {
			return err
		}
	}
	return nil
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
