package bucket_mock

import (
	"sync"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
)

// mockBucket is a mock in-memory bucket.
type mockBucket struct {
	id   string
	conf *bucket.Config
	sm   sync.Map
}

// NewMockBucket constructs a new mock bucket for testing.
func NewMockBucket(id string, conf *bucket.Config) bucket.Bucket {
	if conf == nil {
		conf = NewMockBucketConfig(id, 1)
	}
	return &mockBucket{id: id, conf: conf}
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

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (b *mockBucket) PutBlock(data []byte, opts *bucket.PutOpts) (*bucket_event.PutBlock, error) {
	h, err := hash.Sum(hash.HashType_HashType_SHA256, data)
	if err != nil {
		return nil, err
	}
	ref := cid.NewBlockRef(h)
	ms := ref.MarshalString()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	b.sm.Store(ms, dataCopy)
	return &bucket_event.PutBlock{
		BlockCommon: &bucket_event.BlockCommon{
			BucketId: b.id,
			BlockRef: ref,
		},
	}, nil
}

// GetBlock gets a block with a cid reference.
// Note: the block may not be in the specified bucket.
func (b *mockBucket) GetBlock(ref *cid.BlockRef) ([]byte, bool, error) {
	if err := ref.Validate(); err != nil {
		return nil, false, err
	}
	ms := ref.MarshalString()
	datai, ok := b.sm.Load(ms)
	if !ok {
		return nil, false, nil
	}
	return datai.([]byte), true, nil
}

// GetBlockExists checks if a block exists with a cid reference.
// Note: the block may not be in the specified bucket.
func (b *mockBucket) GetBlockExists(ref *cid.BlockRef) (bool, error) {
	if err := ref.Validate(); err != nil {
		return false, err
	}
	ms := ref.MarshalString()
	_, ok := b.sm.Load(ms)
	return ok, nil
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (b *mockBucket) RmBlock(ref *cid.BlockRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	ms := ref.MarshalString()
	b.sm.Delete(ms)
	return nil
}

// _ is a type assertion
var _ bucket.Bucket = ((*mockBucket)(nil))
