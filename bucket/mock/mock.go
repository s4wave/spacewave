package bucket_mock

import (
	"github.com/aperturerobotics/hydra/block"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	"github.com/aperturerobotics/hydra/bucket"
)

// mockBucket is a mock in-memory bucket.
type mockBucket struct {
	block.Store
	id   string
	conf *bucket.Config
}

// NewMockBucket constructs a new mock bucket for testing.
func NewMockBucket(id string, conf *bucket.Config) bucket.Bucket {
	if conf == nil {
		conf = NewMockBucketConfig(id, 1)
	}
	return &mockBucket{id: id, conf: conf, Store: block_mock.NewMockStore()}
}

// NewMockBucketConfig constructs a new mock bucket config.
func NewMockBucketConfig(id string, version uint32) *bucket.Config {
	return &bucket.Config{
		Id:      id,
		Version: version,
	}
}

// GetBucketConfig returns a copy of the bucket configuration.
func (b *mockBucket) GetBucketConfig() *bucket.Config {
	return b.conf
}

// _ is a type assertion
var _ bucket.Bucket = ((*mockBucket)(nil))
