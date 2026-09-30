package world_block

import (
	"context"
	"math"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	trace "github.com/s4wave/spacewave/db/traceutil"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

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
	// Require an existing World root before opening the update.
	if base.GetRootRef() == nil {
		return nil, errors.New("snapshot update requires an existing World root")
	}

	// Trace the snapshot update for the whole operation.
	ctx, task := trace.NewTask(ctx, "hydra/world-block/update-snapshot")
	defer task.End()

	// Run the update inside one access scope on the base World.
	var result *bucket.ObjectRef
	err := storage.AccessWorldState(ctx, base, func(bucketCursor *bucket_lookup.Cursor) error {
		// Retain writes until the final indexes exist, since copy-on-write
		// replaces intermediate blocks. Sync still sends bounded batches
		// through the destination's normal RPC, GC and durability path.
		writes := block.NewBufferedStoreWithSettings(ctx, bucketCursor.GetBucket(), &block.BufferedStoreSettings{
			MaxPendingEntries:       math.MaxInt,
			MaxPendingBytes:         math.MaxInt,
			MaxPendingMetadataBytes: math.MaxInt,
			DrainBatchEntries:       4096,
		})
		bucketCursor.SetTransactionStore(writes)

		// Apply the update callback and compute the new root.
		root, err := updateWorld(ctx, le, bucketCursor, base, update)
		if err != nil {
			return err
		}

		// Sync only the blocks reachable from the new root.
		if _, err := writes.SyncReachable(ctx, root); err != nil {
			return errors.Wrap(err, "sync snapshot")
		}

		// Capture the destination reference with the new root.
		result = bucketCursor.GetRefWithOpArgs()
		result.RootRef = root
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// updateWorld applies update to the World at base and returns the new root.
// The World state and its indexes are released before it returns, so only
// encoded blocks reach the sync.
func updateWorld(
	ctx context.Context,
	le *logrus.Entry,
	bucketCursor *bucket_lookup.Cursor,
	base *bucket.ObjectRef,
	update func(context.Context, *WorldState) error,
) (*block.BlockRef, error) {
	// Open a write transaction at the base root with a World state over it.
	local := world.NewWorldStorageFromCursor(bucketCursor)
	transaction, cursor := bucketCursor.BuildTransactionAtRef(nil, base.GetRootRef())
	state, err := NewWorldState(ctx, le, true, nil, cursor, nil, local, nil, false)
	if err != nil {
		return nil, err
	}
	defer state.Discard()

	// Point the state's local bucket at the destination.
	state.localBucketID = bucketCursor.GetRefWithOpArgs().GetBucketId()

	// Apply the update, commit it, and write the transaction root.
	if err := update(ctx, state); err != nil {
		return nil, err
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
