//go:build !tinygo

package block_store_s3

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"golang.org/x/sync/errgroup"
)

// DeletePackStore deletes every object of the PackStore under prefix.
//
// The packfile entries go first, so the store's readers and writers stop
// finding its packfiles before those are deleted. A missing bucket holds
// nothing, so it counts as deleted. A rerun finishes an interrupted delete.
//
// A PackStore over the prefix with KeepPacks reports that it keeps every
// block, so the decoded-block pool admits reads it serves and reads of caches
// over it. The delete removes those blocks, so it first records the prefix
// deleted, after which no PackStore over it reports keeping blocks and a
// cache's surviving copy of one reads as not durable. It then reports every
// block removed, and again once it ends, as a read during the delete may admit
// a block it then removes. Each report is one constant-time generation bump.
// The record lasts for the process: a store over a deleted prefix stays out
// of the pool, which costs reuse and never correctness.
func DeletePackStore(ctx context.Context, client ObjectStore, bucket, prefix string) error {
	// Stop the store vouching for its blocks, then report them removed, now
	// and once the delete ends.
	markPackStoreDeleted(bucket, prefix)
	block.InvalidateAllRemovedBlocks()
	defer block.InvalidateAllRemovedBlocks()

	// Delete the packfile entries, then every object under the prefix.
	for _, dir := range []string{prefix + entryDir, prefix} {
		if err := deleteObjectsUnder(ctx, client, bucket, dir); err != nil {
			if errors.Is(err, ErrBucketNotFound) {
				return nil
			}
			return err
		}
	}
	return nil
}

// packStoreKey names the PackStore of a bucket and prefix.
type packStoreKey struct {
	bucket, prefix string
}

// deletedPackStores holds a packStoreKey for each PackStore DeletePackStore
// began to delete, and anyDeleted is set once it holds one, so a store asks
// the map only after a delete.
var (
	deletedPackStores sync.Map
	anyDeleted        atomic.Bool
)

// markPackStoreDeleted records that the PackStore of bucket and prefix is
// being deleted.
func markPackStoreDeleted(bucket, prefix string) {
	deletedPackStores.Store(packStoreKey{bucket: bucket, prefix: prefix}, struct{}{})
	anyDeleted.Store(true)
}

// packStoreDeleted reports whether DeletePackStore began to delete the
// PackStore of bucket and prefix.
func packStoreDeleted(bucket, prefix string) bool {
	if !anyDeleted.Load() {
		return false
	}
	_, ok := deletedPackStores.Load(packStoreKey{bucket: bucket, prefix: prefix})
	return ok
}

// deleteObjectsUnder deletes every object listed under prefix.
func deleteObjectsUnder(ctx context.Context, client ObjectStore, bucket, prefix string) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchConcurrency)
	eg.Go(func() error {
		return client.ListObjects(egCtx, bucket, prefix, func(key string, _ int64) error {
			eg.Go(func() error {
				return client.DeleteObject(egCtx, bucket, key)
			})
			return nil
		})
	})
	return errors.Wrap(eg.Wait(), "delete "+prefix)
}
