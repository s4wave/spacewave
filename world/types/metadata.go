package world_types

import (
	"context"
	"strings"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/path"
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
	for i, key := range keys {
		result[i] = &ObjectMetadata{ObjectKey: key}
	}

	err := ws.AccessCayleyGraph(ctx, false, func(ctx context.Context, h world.CayleyHandle) error {
		for i, key := range keys {
			gv := world.KeyToGraphValue(key)

			// Type lookup: subject.Out(TypePred) uses (subject,predicate) index.
			if err := lookupTypeBatch(ctx, h, gv, result[i]); err != nil {
				return err
			}

			// Parent lookup: subject+ParentPred filter uses (subject,predicate) index.
			if err := lookupParentBatch(ctx, h, gv, result[i]); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// lookupTypeBatch resolves the type for a single object within an open graph handle.
func lookupTypeBatch(ctx context.Context, h world.CayleyHandle, gv quad.Value, md *ObjectMetadata) error {
	it := path.StartPath(h, gv).
		Out(TypePred).
		BuildIterator(ctx).
		Iterate(ctx)
	defer it.Close()

	for it.Next(ctx) && md.TypeID == "" {
		res, err := it.Result(ctx)
		if err != nil {
			return err
		}
		qv, err := h.NameOf(ctx, res)
		if err != nil {
			return err
		}
		key, err := world.QuadValueToKey(qv)
		if err != nil {
			return err
		}
		if strings.HasPrefix(key, TypesPrefix) {
			md.TypeID = key[len(TypesPrefix):]
		}
	}
	return it.Err()
}

// lookupParentBatch resolves the parent for a single object within an open graph handle.
func lookupParentBatch(ctx context.Context, h world.CayleyHandle, gv quad.Value, md *ObjectMetadata) error {
	return world.FilterIterateQuads(ctx, h, quad.Quad{
		Subject:   gv,
		Predicate: world_parent.ParentPred,
	}, func(q quad.Quad) error {
		if q.Object != nil {
			parentKey, err := world.QuadValueToKey(q.Object)
			if err != nil {
				return err
			}
			md.ParentObjectKey = parentKey
		}
		return nil
	})
}
