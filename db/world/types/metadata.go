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

// GetObjectMetadataBatch returns the type and parent metadata for a list of
// object keys in one batched graph lookup. It avoids the local Cayley handle
// API so remote and TinyGo callers use the same path, and a remote World
// answers it in one call.
//
// The result slice preserves the input key order.
func GetObjectMetadataBatch(ctx context.Context, ws world.WorldState, keys []string) ([]*ObjectMetadata, error) {
	// Return no metadata when the caller supplies no object keys.
	if len(keys) == 0 {
		return nil, nil
	}

	// Preserve input order while grouping metadata records by unique object
	// key, and ask for the type and parent edges of each unique key.
	result := make([]*ObjectMetadata, len(keys))
	resultByKey := make(map[string][]*ObjectMetadata, len(keys))
	filters := make([]world.GraphQuad, 0, 2*len(keys))
	for i, key := range keys {
		md := &ObjectMetadata{ObjectKey: key}
		result[i] = md
		if _, ok := resultByKey[key]; !ok {
			filters = append(filters,
				world.NewGraphQuadWithKeys(key, TypePred.String(), "", ""),
				world.NewGraphQuadWithKeys(key, world_parent.ParentPred.String(), "", ""),
			)
		}
		resultByKey[key] = append(resultByKey[key], md)
	}

	// Apply each key's type edges, then its parent edges.
	quads, err := ws.LookupGraphQuadsBatch(ctx, filters, typeGraphLookupLimit)
	if err != nil {
		return nil, err
	}
	for i := 0; i+1 < len(quads); i += 2 {
		for _, q := range quads[i] {
			if err := setTypeBatch(q, resultByKey); err != nil {
				return nil, err
			}
		}
		for _, q := range quads[i+1] {
			if err := setParentBatch(q, resultByKey); err != nil {
				return nil, err
			}
		}
	}
	return result, nil
}

// setTypeBatch updates result metadata from a type quad.
func setTypeBatch(q world.GraphQuad, resultByKey map[string][]*ObjectMetadata) error {
	// Ignore graph quads without both object endpoints.
	if q.GetSubject() == "" || q.GetObj() == "" {
		return nil
	}

	// Decode the graph subject into the metadata object key.
	objKey, err := world.GraphValueToKey(q.GetSubject())
	if err != nil {
		return err
	}

	// Decode the type endpoint and require the type object prefix.
	typeKey, err := world.GraphValueToKey(q.GetObj())
	if err != nil {
		return err
	}
	if !strings.HasPrefix(typeKey, TypesPrefix) {
		return nil
	}

	// Fill unset type identifiers in the matching metadata records.
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
	// Ignore graph quads without both object endpoints.
	if q.GetSubject() == "" || q.GetObj() == "" {
		return nil
	}

	// Decode the graph subject into the metadata object key.
	objKey, err := world.GraphValueToKey(q.GetSubject())
	if err != nil {
		return err
	}

	// Decode the parent endpoint into its object key.
	parentKey, err := world.GraphValueToKey(q.GetObj())
	if err != nil {
		return err
	}

	// Record the parent object key in every matching metadata record.
	for _, md := range resultByKey[objKey] {
		md.ParentObjectKey = parentKey
	}
	return nil
}
