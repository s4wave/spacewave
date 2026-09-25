//go:build !tinygo

package block_store_s3

import (
	"context"

	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// DeletePackStore deletes every object of the PackStore under prefix.
//
// The packfile entries go first, so the store's readers and writers stop
// finding its packfiles before those are deleted. A missing bucket holds
// nothing, so it counts as deleted. A rerun finishes an interrupted delete.
func DeletePackStore(ctx context.Context, client *Client, bucket, prefix string) error {
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
func deleteObjectsUnder(ctx context.Context, client *Client, bucket, prefix string) error {
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchConcurrency)
	eg.Go(func() error {
		return client.ListObjects(egCtx, bucket, prefix, func(key string, _ int64) error {
			eg.Go(func() error {
				err := client.DeleteObject(egCtx, bucket, key)
				if errors.Is(err, ErrNotFound) {
					return nil
				}
				return err
			})
			return nil
		})
	})
	return errors.Wrap(eg.Wait(), "delete "+prefix)
}
