package world_block

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
)

// ChangedObjectKeys lists the objects whose records differ between two World
// roots. It compares the object trees block by block and skips every subtree
// the roots share, so its cost follows the size of the change rather than the
// size of the World. complete is false when the change exceeds maxKeys or the
// comparison bound; the caller must then treat every object as changed.
func ChangedObjectKeys(ctx context.Context, before, after *block.Cursor, maxKeys int) ([]string, bool, error) {
	// Decode both World roots so their object tree sub-blocks resolve.
	for _, bcs := range []*block.Cursor{before, after} {
		if _, err := UnmarshalWorld(ctx, bcs); err != nil {
			return nil, false, err
		}
	}

	// Compare the object trees, which hold one record per object key.
	keys, complete, err := kvtx_block.ChangedKeys(ctx, before.FollowSubBlock(1), after.FollowSubBlock(1), maxKeys)
	if err != nil || !complete {
		return nil, false, err
	}

	// Strip the storage prefix to recover the object keys.
	objKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if objKey, ok := bytes.CutPrefix(key, []byte(objectKeyPrefix)); ok {
			objKeys = append(objKeys, string(objKey))
		}
	}
	return objKeys, true, nil
}
