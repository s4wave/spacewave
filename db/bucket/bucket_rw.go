package bucket

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// bucketRW combines a read and write bucket together.
type bucketRW struct {
	block.StoreOps
	conf *Config
}

// NewBucketRW constructs a new Bucket handle using a read handle and an
// optional write handle. If the write handle is not nil, the write (put and
// delete) calls will go to it. Otherwise, all calls are sent to the read
// handle.
func NewBucketRW(readHandle Bucket, writeHandle BucketOps) Bucket {
	if writeHandle == nil {
		writeHandle = readHandle
	}
	return &bucketRW{
		StoreOps: block.NewStoreRW(readHandle, writeHandle),
		conf:     readHandle.GetBucketConfig(),
	}
}

// GetBucketConfig returns a copy of the bucket configuration.
func (b *bucketRW) GetBucketConfig() *Config {
	return b.conf
}

// PutBlockBatch forwards batched writes to the inner StoreOps when supported.
func (b *bucketRW) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	batcher, ok := b.StoreOps.(block.BatchPutStore)
	if !ok {
		for _, entry := range entries {
			if entry.Tombstone {
				if err := b.RmBlock(ctx, entry.Ref); err != nil {
					return err
				}
				continue
			}
			if _, _, err := b.PutBlock(ctx, entry.Data, &block.PutOpts{
				ForceBlockRef: entry.Ref.Clone(),
			}); err != nil {
				return err
			}
		}
		return nil
	}
	return batcher.PutBlockBatch(ctx, entries)
}

// GetBlockExistsBatch forwards batched existence probes to the inner StoreOps when supported.
func (b *bucketRW) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if batcher, ok := b.StoreOps.(block.BatchExistsStore); ok {
		return batcher.GetBlockExistsBatch(ctx, refs)
	}

	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := b.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// PutBlockBackground forwards background writes to the inner StoreOps when supported.
func (b *bucketRW) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	bg, ok := b.StoreOps.(block.BackgroundPutStore)
	if !ok {
		return b.PutBlock(ctx, data, opts)
	}
	return bg.PutBlockBackground(ctx, data, opts)
}

// BeginDeferFlush forwards to the inner StoreOps if it supports deferred flushing.
func (b *bucketRW) BeginDeferFlush() {
	if df, ok := b.StoreOps.(block.DeferFlushable); ok {
		df.BeginDeferFlush()
	}
}

// EndDeferFlush forwards to the inner StoreOps if it supports deferred flushing.
func (b *bucketRW) EndDeferFlush(ctx context.Context) error {
	if df, ok := b.StoreOps.(block.DeferFlushable); ok {
		return df.EndDeferFlush(ctx)
	}
	return nil
}

// _ is a type assertion
var (
	_ Bucket                   = ((*bucketRW)(nil))
	_ block.BatchExistsStore   = ((*bucketRW)(nil))
	_ block.BatchPutStore      = ((*bucketRW)(nil))
	_ block.BackgroundPutStore = ((*bucketRW)(nil))
	_ block.DeferFlushable     = ((*bucketRW)(nil))
)
