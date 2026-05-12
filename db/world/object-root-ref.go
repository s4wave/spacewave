package world

import (
	"context"

	"github.com/s4wave/spacewave/db/bucket"
)

// ObjectRootRef holds the current root reference metadata for one object key.
type ObjectRootRef struct {
	// ObjectKey is the object key.
	ObjectKey string
	// RootRef is the current object root ref.
	RootRef *bucket.ObjectRef
	// Rev is the object revision.
	Rev uint64
	// Exists is true when the object exists.
	Exists bool
}

// ObjectRootRefBatcher returns object root refs for object keys.
type ObjectRootRefBatcher interface {
	// GetObjectRootRefsBatch returns object root refs for object keys.
	GetObjectRootRefsBatch(ctx context.Context, keys []string) ([]*ObjectRootRef, error)
}

// GetObjectRootRefsBatch returns object root refs for object keys.
func GetObjectRootRefsBatch(ctx context.Context, ws WorldState, keys []string) ([]*ObjectRootRef, error) {
	if len(keys) == 0 {
		return nil, nil
	}
	if batcher, ok := ws.(ObjectRootRefBatcher); ok {
		return batcher.GetObjectRootRefsBatch(ctx, keys)
	}

	out := make([]*ObjectRootRef, len(keys))
	for i, key := range keys {
		ref := &ObjectRootRef{ObjectKey: key}
		out[i] = ref
		obj, exists, err := ws.GetObject(ctx, key)
		if err != nil {
			return nil, err
		}
		ref.Exists = exists
		if !exists {
			continue
		}
		rootRef, rev, err := obj.GetRootRef(ctx)
		if err != nil {
			return nil, err
		}
		ref.RootRef = rootRef
		ref.Rev = rev
	}
	return out, nil
}
