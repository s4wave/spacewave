package bucket

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// bucketRW combines a read and write bucket together.
type bucketRW struct {
	store block.StoreOps
	conf  *Config
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
		store: block.NewStoreRW(readHandle, writeHandle),
		conf:  readHandle.GetBucketConfig(),
	}
}

// GetBucketConfig returns a copy of the bucket configuration.
func (b *bucketRW) GetBucketConfig() *Config {
	return b.conf
}

// GetHashType returns the preferred hash type for the store.
func (b *bucketRW) GetHashType() hash.HashType {
	return b.store.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (b *bucketRW) GetSupportedFeatures() block.StoreFeature {
	return b.store.GetSupportedFeatures()
}

// BeginReadOperation opens a read scope on the inner store.
func (b *bucketRW) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := b.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	scoped := *b
	scoped.store = store
	return &scoped, release, nil
}

// PutBlock forwards to the inner store.
func (b *bucketRW) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.store.PutBlock(ctx, data, opts)
}

// PutBlockBatch forwards batched writes to the inner StoreOps.
func (b *bucketRW) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return b.store.PutBlockBatch(ctx, entries)
}

// GetBlock forwards to the inner store.
func (b *bucketRW) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return b.store.GetBlock(ctx, ref)
}

// GetBlockExists forwards to the inner store.
func (b *bucketRW) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return b.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards batched existence probes to the inner StoreOps.
func (b *bucketRW) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return b.store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock forwards to the inner store.
func (b *bucketRW) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return b.store.RmBlock(ctx, ref)
}

// StatBlock forwards to the inner store.
func (b *bucketRW) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return b.store.StatBlock(ctx, ref)
}

// Sync forwards the durability barrier to the inner store.
func (b *bucketRW) Sync(ctx context.Context) (bool, error) {
	return b.store.Sync(ctx)
}

// BeginDeferFlush forwards the GC defer-flush scope to the inner store.
func (b *bucketRW) BeginDeferFlush() {
	block.BeginDeferFlush(b.store)
}

// EndDeferFlush forwards closing the GC defer-flush scope to the inner store.
func (b *bucketRW) EndDeferFlush(ctx context.Context) error {
	return block.EndDeferFlush(ctx, b.store)
}

// _ is a type assertion
var (
	_ Bucket = ((*bucketRW)(nil))
)
