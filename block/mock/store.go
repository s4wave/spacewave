package block_mock

import (
	"context"
	"sync"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
)

// mockStore is a mock in-memory store.
type mockStore struct {
	sm       sync.Map
	hashType hash.HashType
}

// NewMockStore constructs a new mock bucket for testing.
//
// hashType is the hash type to use, 0 for default.
func NewMockStore(hashType hash.HashType) block.Store {
	return &mockStore{hashType: hashType}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (b *mockStore) GetHashType() hash.HashType {
	return b.hashType
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (b *mockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	hashType := opts.GetHashType()
	if hashType == 0 {
		hashType = b.hashType
	}
	if hashType == 0 {
		hashType = block.DefaultHashType
	}
	h, err := hash.Sum(hashType, data)
	if err != nil {
		return nil, false, err
	}
	ref := block.NewBlockRef(h)
	ms := ref.MarshalString()
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	b.sm.Store(ms, dataCopy)
	return ref, false, nil
}

// GetBlock gets a block with a cid reference.
// Note: the block may not be in the specified bucket.
func (b *mockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
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
func (b *mockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
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
func (b *mockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	ms := ref.MarshalString()
	b.sm.Delete(ms)
	return nil
}

// _ is a type assertion
var _ block.Store = ((*mockStore)(nil))
