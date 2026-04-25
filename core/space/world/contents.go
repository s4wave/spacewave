package space_world

import (
	"context"
	"strings"

	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// BuildWorldContents builds the list of world objects.
func BuildWorldContents(ctx context.Context, ws world.WorldState) (*WorldContents, error) {
	it := ws.IterateObjects(ctx, "", false)
	defer it.Close()

	var objKeys []string
	var types []*WorldContentsObjectType
	for it.Next() {
		key := it.Key()
		if strings.HasPrefix(key, world_types.TypesPrefix) {
			types = append(types, &WorldContentsObjectType{ObjectType: key[len(world_types.TypesPrefix):]})
			continue
		}
		objKeys = append(objKeys, key)
	}
	if err := it.Err(); err != nil {
		return nil, err
	}

	metadata, err := world_types.GetObjectMetadataBatch(ctx, ws, objKeys)
	if err != nil {
		return nil, err
	}

	entries := make([]*WorldContentsObject, len(metadata))
	for i, md := range metadata {
		entries[i] = &WorldContentsObject{
			ObjectKey:       md.ObjectKey,
			ParentObjectKey: md.ParentObjectKey,
			ObjectType:      md.TypeID,
		}
	}

	return &WorldContents{Objects: entries, ObjectTypes: types}, nil
}
