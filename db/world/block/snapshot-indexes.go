package world_block

import (
	"context"

	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// packSnapshotGraph builds the graph index for a fresh bulk import.
func (t *WorldState) packSnapshotGraph(ctx context.Context, index *hashmap.BTreeMap[[]byte]) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/build-snapshot/pack-graph")
	defer task.End()

	if t.graphTree != nil {
		t.graphTree.Discard()
		t.graphTree = nil
	}
	cursor := t.bcs.Detach(false)
	cursor.SetBlock(kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA_INLINE), true)
	packed, err := kvtx_block.BuildKvTransaction(ctx, cursor, true)
	if err != nil {
		return err
	}
	tree := packed.(*kvtx_block_okra.Tx)
	if err := tree.ReplaceAll(ctx, func(yield func([]byte, []byte) bool) {
		_ = index.Iterate(ctx, func(_ context.Context, key, value []byte) error {
			if !yield(key, value) {
				return context.Canceled
			}
			return nil
		})
	}); err != nil {
		packed.Discard()
		return err
	}
	if err := cursor.SetAsSubBlock(2, t.bcs); err != nil {
		packed.Discard()
		return err
	}
	t.graphTree = packed
	return nil
}
