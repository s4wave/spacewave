package world_block

import (
	"context"
	"slices"

	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	kvtx_cayley "github.com/s4wave/spacewave/db/kvtx/cayley"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	trace "github.com/s4wave/spacewave/db/traceutil"
)

// stageSnapshotObjects gives immutable packed metadata a mutable index for the
// update. Object bodies remain lazy; only final metadata is packed and written.
func (t *WorldState) stageSnapshotObjects(ctx context.Context) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/update-snapshot/stage-objects")
	defer task.End()

	cursor := t.bcs.Detach(false)
	cursor.SetBlock(kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL), true)
	staged, err := kvtx_block.BuildKvTransaction(ctx, cursor, true)
	if err != nil {
		return err
	}
	previous := t.objTree
	iterator := previous.BlockIterate(ctx, nil, true, false)
	defer iterator.Close()
	if err := iterator.Seek(nil); err != nil {
		staged.Discard()
		return err
	}
	for iterator.Valid() {
		if err := ctx.Err(); err != nil {
			staged.Discard()
			return err
		}
		if err := staged.SetCursorAtKey(ctx, slices.Clone(iterator.Key()), iterator.ValueCursor(), false); err != nil {
			staged.Discard()
			return err
		}
		iterator.Next()
	}
	if err := iterator.Err(); err != nil {
		staged.Discard()
		return err
	}
	if err := cursor.SetAsSubBlock(1, t.bcs); err != nil {
		staged.Discard()
		return err
	}
	t.objTree = staged
	previous.Discard()
	return nil
}

// stageSnapshotGraph retains the existing Cayley index in memory so each graph
// delta does not rebuild immutable pages. World graph APIs still own semantics.
func (t *WorldState) stageSnapshotGraph(ctx context.Context) (*hashmap.BTreeMap[[]byte], error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/update-snapshot/stage-graph")
	defer task.End()

	index := hashmap.NewBTreeMap[[]byte]()
	if err := t.graphTree.ScanPrefix(ctx, nil, func(key, value []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return index.Set(ctx, key, slices.Clone(value))
	}); err != nil {
		return nil, err
	}
	staged, err := kvtx_cayley.NewGraph(ctx, hashmap.NewHashmapKvtx(index), graph.Options{cayley_kv.OptAssumeDefaultIdx: true})
	if err != nil {
		return nil, err
	}
	_ = t.graphHd.Close()
	t.graphTree.Discard()
	t.graphHd = staged
	// No block index exists for the mutable graph until final packing.
	t.graphTree = nil
	return index, nil
}

// packSnapshotGraph materializes the updated Cayley keys once at completion.
func (t *WorldState) packSnapshotGraph(ctx context.Context, index *hashmap.BTreeMap[[]byte]) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/update-snapshot/pack-graph")
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
