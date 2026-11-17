//go:build !tinygo

package block_store_s3

import (
	"context"

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
// over it. The delete removes those blocks, so it reports every block removed
// first, and again once it ends, as a read during the delete may admit a block
// it then removes. Each report is one constant-time generation bump.
func DeletePackStore(ctx context.Context, client ObjectStore, bucket, prefix string) error {
	block.InvalidateAllRemovedBlocks()
	defer block.InvalidateAllRemovedBlocks()
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
