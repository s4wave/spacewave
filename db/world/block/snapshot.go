package world_block

import (
	"context"
	"math"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// BuildSnapshot constructs an independent World in memory, then writes and
// syncs its final structure. Populate uses the ordinary World APIs; it must not
// retain the state or its cursors. The caller owns publication of the returned
// reference in its enclosing World transaction.
//
// Construction retains encoded blocks and the mutable object index in memory.
// Use this for finite snapshots that fit in memory. No existing World is
// replaced, and no snapshot reference is returned on failure. Individual change
// records are omitted; object revisions and the World sequence are preserved.
func BuildSnapshot(
	ctx context.Context,
	le *logrus.Entry,
	storage world.WorldStorage,
	populate func(context.Context, *WorldState) error,
) (*bucket.ObjectRef, error) {
	return writeSnapshot(ctx, le, storage, nil, populate)
}

// UpdateSnapshot applies ordinary World mutations to copy-on-write indexes and
// syncs only final reachable new blocks. Unchanged blocks and the prior root
// remain readable. The caller publishes the returned root in its enclosing
// transaction. Update must not retain the state or its cursors. Encoded changes
// are buffered in memory until the final root is known.
func UpdateSnapshot(
	ctx context.Context,
	le *logrus.Entry,
	storage world.WorldStorage,
	base *bucket.ObjectRef,
	update func(context.Context, *WorldState) error,
) (*bucket.ObjectRef, error) {
	if base.GetRootRef() == nil {
		return nil, errors.New("snapshot update requires an existing World root")
	}
	return writeSnapshot(ctx, le, storage, base, update)
}

// writeSnapshot shares staging and durable copying for construction and updates.
func writeSnapshot(
	ctx context.Context,
	le *logrus.Entry,
	storage world.WorldStorage,
	base *bucket.ObjectRef,
	populate func(context.Context, *WorldState) error,
) (*bucket.ObjectRef, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/build-snapshot")
	defer task.End()

	var result *bucket.ObjectRef
	err := storage.AccessWorldState(ctx, base, func(bucketCursor *bucket_lookup.Cursor) error {
		// Retain writes until the final indexes exist. Sync still sends bounded
		// batches through the destination's normal RPC, GC and durability path.
		writes := block.NewBufferedStoreWithSettings(ctx, bucketCursor.GetBucket(), &block.BufferedStoreSettings{
			MaxPendingEntries:       math.MaxInt,
			MaxPendingBytes:         math.MaxInt,
			MaxPendingMetadataBytes: math.MaxInt,
			DrainBatchEntries:       4096,
		})
		bucketCursor.SetTransactionStore(writes)
		root, err := buildSnapshot(ctx, le, bucketCursor, base.GetRootRef(), populate)
		if err != nil {
			return err
		}
		if _, err := writes.SyncReachable(ctx, root); err != nil {
			return errors.Wrap(err, "sync snapshot")
		}
		result = bucketCursor.GetRefWithOpArgs()
		result.RootRef = root
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// buildSnapshot finishes and releases mutable construction state before the
// caller begins durable copying. Only encoded reachable blocks cross that fence.
func buildSnapshot(
	ctx context.Context,
	le *logrus.Entry,
	bucketCursor *bucket_lookup.Cursor,
	base *block.BlockRef,
	populate func(context.Context, *WorldState) error,
) (*block.BlockRef, error) {
	local := world.NewWorldStorageFromCursor(bucketCursor)
	transaction, cursor := bucketCursor.BuildTransactionAtRef(nil, base)
	if base == nil {
		cursor.SetBlock(NewWorld(true), true)
	}
	state, err := NewWorldState(ctx, le, true, nil, cursor, nil, nil, nil, local, nil, false)
	if err != nil {
		return nil, err
	}
	defer state.Discard()
	if base != nil {
		state.localBucketID = bucketCursor.GetRefWithOpArgs().GetBucketId()
	}

	if err := populate(ctx, state); err != nil {
		return nil, err
	}
	if base == nil {
		if err := state.packSnapshotObjects(ctx, bucketCursor.GetRefWithOpArgs().GetBucketId()); err != nil {
			return nil, errors.Wrap(err, "build snapshot object index")
		}
	}
	if err := state.Commit(ctx); err != nil {
		return nil, err
	}
	root, _, err := transaction.Write(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "write snapshot")
	}
	return root, nil
}

// localObjectRef keeps same-bucket bodies in the World DAG.
func (t *WorldState) localObjectRef(ref *bucket.ObjectRef) *bucket.ObjectRef {
	if t.localBucketID == "" || ref.GetRootRef() == nil || ref.GetBucketId() != t.localBucketID {
		return ref
	}
	local := ref.Clone()
	local.BucketId = ""
	return local
}

// externalObjectRef restores the current bucket on a local World DAG edge.
func (t *WorldState) externalObjectRef(ref *bucket.ObjectRef) *bucket.ObjectRef {
	if t.localBucketID == "" || ref.GetRootRef() == nil || ref.GetBucketId() != "" {
		return ref
	}
	external := ref.Clone()
	external.BucketId = t.localBucketID
	return external
}

// packSnapshotObjects replaces the temporary mutable index with one packed
// tree. Only final object values are materialized; intermediate AVL nodes are
// never written. Ordinary World mutations own object and revision semantics.
func (t *WorldState) packSnapshotObjects(ctx context.Context, bucketID string) error {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/build-snapshot/pack-objects")
	defer task.End()

	packedCursor := t.bcs.Detach(false)
	packedCursor.SetBlock(kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA), true)
	packed, err := kvtx_block.BuildKvTransaction(ctx, packedCursor, true)
	if err != nil {
		return err
	}
	tree := packed.(*kvtx_block_okra.Tx)
	iterator := t.objTree.BlockIterate(ctx, nil, true, false)
	defer iterator.Close()
	if err := iterator.Seek(nil); err != nil {
		return err
	}
	var keys [][]byte
	var values []*block.Cursor
	for iterator.Valid() {
		if err := ctx.Err(); err != nil {
			return err
		}
		value := iterator.ValueCursor()
		object, err := UnmarshalObject(ctx, value)
		if err != nil {
			return err
		}
		// A body in this bucket is a local edge in the self-contained DAG.
		// Preserve its transformer and revision while making that edge
		// visible to ordinary block traversal and reference accounting.
		if ref := object.GetRootRef(); bucketID != "" && ref.GetBucketId() == bucketID {
			object = object.Clone()
			object.RootRef.BucketId = ""
			value.SetBlock(object, true)
		}
		keys = append(keys, slices.Clone(iterator.Key()))
		values = append(values, value)
		iterator.Next()
	}
	if err := iterator.Err(); err != nil {
		return err
	}
	// Final metadata shares one transaction pass; building a transaction for
	// each leaf repeats graph setup, scheduling, and buffer bookkeeping.
	if err := t.bcs.GetTransaction().WriteAtRoots(ctx, values); err != nil {
		return err
	}
	if err := tree.ReplaceAllCursors(ctx, func(yield func([]byte, *block.Cursor) bool) {
		for i, value := range values {
			if !yield(keys[i], value) {
				return
			}
		}
	}, false); err != nil {
		return err
	}
	// Release the mutable index and its value cursors once the packed tree
	// replaces it. Only the packed tree's blocks are written.
	mutable := t.bcs.FollowSubBlock(1)
	if err := packedCursor.SetAsSubBlock(1, t.bcs); err != nil {
		return err
	}
	t.objTree.Discard()
	t.objTree = packed
	mutable.DiscardDetachedTree()
	return nil
}
