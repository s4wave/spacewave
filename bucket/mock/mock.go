package bucket_mock

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/bucket"
)

// mockBucket is a mock in-memory bucket.
type mockBucket struct {
	store block.StoreOps
	conf *bucket.Config
}

// NewMockBucket constructs a new mock bucket for testing.
func NewMockBucket(id string, conf *bucket.Config) bucket.Bucket {
	if conf == nil {
		conf = NewMockBucketConfig(id, 1)
	}
	return &mockBucket{
		store: block_mock.NewMockStore(conf.GetPutOpts().GetHashType()),
		conf:  conf,
	}
}

// NewMockBucketConfig constructs a new mock bucket config.
func NewMockBucketConfig(id string, rev uint32) *bucket.Config {
	return &bucket.Config{
		Id:  id,
		Rev: rev,
	}
}

// GetBucketConfig returns a copy of the bucket configuration.
func (b *mockBucket) GetBucketConfig() *bucket.Config {
	return b.conf
}

// GetHashType returns the preferred hash type.
func (b *mockBucket) GetHashType() hash.HashType {
	return b.store.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask.
func (b *mockBucket) GetSupportedFeatures() block.StoreFeature {
	return b.store.GetSupportedFeatures()
}

// PutBlock forwards to the inner store.
func (b *mockBucket) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.store.PutBlock(ctx, data, opts)
}

// PutBlockBatch forwards to the inner store.
func (b *mockBucket) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return b.store.PutBlockBatch(ctx, entries)
}

// PutBlockBackground forwards to the inner store.
func (b *mockBucket) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return b.store.PutBlockBackground(ctx, data, opts)
}

// GetBlock forwards to the inner store.
func (b *mockBucket) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return b.store.GetBlock(ctx, ref)
}

// GetBlockExists forwards to the inner store.
func (b *mockBucket) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return b.store.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards to the inner store.
func (b *mockBucket) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return b.store.GetBlockExistsBatch(ctx, refs)
}

// RmBlock forwards to the inner store.
func (b *mockBucket) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return b.store.RmBlock(ctx, ref)
}

// StatBlock forwards to the inner store.
func (b *mockBucket) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return b.store.StatBlock(ctx, ref)
}

// Flush forwards to the inner store.
func (b *mockBucket) Flush(ctx context.Context) error {
	return b.store.Flush(ctx)
}

// BeginDeferFlush forwards to the inner store.
func (b *mockBucket) BeginDeferFlush() {
	b.store.BeginDeferFlush()
}

// EndDeferFlush forwards to the inner store.
func (b *mockBucket) EndDeferFlush(ctx context.Context) error {
	return b.store.EndDeferFlush(ctx)
}

// _ is a type assertion
var _ bucket.Bucket = ((*mockBucket)(nil))
