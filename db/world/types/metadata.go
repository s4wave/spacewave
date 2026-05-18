package world_types

import (
	"context"
	"strings"

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

// ObjectMetadataBatcher returns metadata for object keys without requiring a
// Cayley handle.
type ObjectMetadataBatcher interface {
	// GetObjectMetadataBatch returns graph metadata for object keys.
	GetObjectMetadataBatch(ctx context.Context, keys []string) ([]*ObjectMetadata, error)
}

// GetObjectMetadataBatch returns the type and parent metadata for a list of
// object keys using exact graph lookups. This intentionally avoids the local
// Cayley handle API so remote and TinyGo callers can use the same path.
//
// The result slice preserves the input key order.
func GetObjectMetadataBatch(ctx context.Context, ws world.WorldState, keys []string) ([]*ObjectMetadata, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if batcher, ok := ws.(ObjectMetadataBatcher); ok {
		return batcher.GetObjectMetadataBatch(ctx, keys)
	}

	result := make([]*ObjectMetadata, len(keys))
	uniqueKeys := make([]string, 0, len(keys))
	seen := make(map[string]struct{}, len(keys))
	resultByKey := make(map[string][]*ObjectMetadata, len(keys))
	for i, key := range keys {
		md := &ObjectMetadata{ObjectKey: key}
		result[i] = md
		if _, ok := seen[key]; !ok {
			seen[key] = struct{}{}
			uniqueKeys = append(uniqueKeys, key)
		}
		resultByKey[key] = append(resultByKey[key], md)
	}

	for _, key := range uniqueKeys {
		typeQuads, err := ws.LookupGraphQuads(
			ctx,
			world.NewGraphQuadWithKeys(key, TypePred.String(), "", ""),
			typeGraphLookupLimit,
		)
		if err != nil {
			return nil, err
		}
		for _, q := range typeQuads {
			if err := setTypeBatch(q, resultByKey); err != nil {
				return nil, err
			}
		}

		parentQuads, err := ws.LookupGraphQuads(
			ctx,
			world.NewGraphQuadWithKeys(key, world_parent.ParentPred.String(), "", ""),
			typeGraphLookupLimit,
		)
		if err != nil {
			return nil, err
		}
		for _, q := range parentQuads {
			if err := setParentBatch(q, resultByKey); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

// setTypeBatch updates result metadata from a type quad.
func setTypeBatch(q world.GraphQuad, resultByKey map[string][]*ObjectMetadata) error {
	if q.GetSubject() == "" || q.GetObj() == "" {
		return nil
	}

	objKey, err := world.GraphValueToKey(q.GetSubject())
	if err != nil {
		return err
	}
	typeKey, err := world.GraphValueToKey(q.GetObj())
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
func setParentBatch(q world.GraphQuad, resultByKey map[string][]*ObjectMetadata) error {
	if q.GetSubject() == "" || q.GetObj() == "" {
		return nil
	}

	objKey, err := world.GraphValueToKey(q.GetSubject())
	if err != nil {
		return err
	}
	parentKey, err := world.GraphValueToKey(q.GetObj())
	if err != nil {
		return err
	}
	for _, md := range resultByKey[objKey] {
		md.ParentObjectKey = parentKey
	}
	return nil
}
