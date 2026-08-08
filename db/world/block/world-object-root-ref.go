package world_block

import (
	"context"

	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// GetObjectRootRefsBatch returns object root refs for object keys.
func (t *WorldState) GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*world.ObjectRootRef, error) {
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}
	out := make([]*world.ObjectRootRef, len(keys))
	for i, key := range keys {
		ref := &world.ObjectRootRef{ObjectKey: key}
		out[i] = ref
		obj, exists, err := t.getObjectRootRef(ctx, key)
		if err != nil {
			return nil, err
		}
		ref.Exists = exists
		if !exists {
			continue
		}
		ref.RootRef = obj.GetRootRef()
		ref.Rev = obj.GetRev()
	}
	return out, nil
}

func (t *WorldState) getObjectRootRef(ctx context.Context, key string) (*Object, bool, error) {
	ot := t.objTree
	k := []byte(objectKeyPrefix + key)
	bcs, err := ot.GetCursorAtKey(ctx, k)
	if err != nil || bcs == nil {
		return nil, false, err
	}
	obj, err := UnmarshalObject(ctx, bcs)
	if err != nil || obj == nil {
		return nil, false, err
	}
	return obj, true, nil
}

// _ is a type assertion
var _ world.ObjectRootRefBatcher = (*WorldState)(nil)
