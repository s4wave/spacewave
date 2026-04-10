//go:build js

package blockshard

import (
	"context"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
	b58 "github.com/mr-tron/base58/base58"
)

// BlockStore wraps the shard engine to implement block.StoreOps.
type BlockStore struct {
	engine   *Engine
	hashType hash.HashType
}

// NewBlockStore creates a block.StoreOps backed by the shard engine.
func NewBlockStore(engine *Engine, hashType hash.HashType) *BlockStore {
	return &BlockStore{engine: engine, hashType: hashType}
}

// GetHashType returns the preferred hash type.
func (s *BlockStore) GetHashType() hash.HashType {
	return s.hashType
}

// PutBlock puts a block into the store.
func (s *BlockStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}

	if opts == nil {
		opts = &block.PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(s.hashType)

	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forceRef := opts.GetForceBlockRef(); !forceRef.GetEmpty() {
		if !ref.EqualsRef(forceRef) {
			return ref, false, block.ErrBlockRefMismatch
		}
	}

	key, err := encodeRef(ref)
	if err != nil {
		return nil, false, err
	}

	// Check if block already exists.
	_, found, err := s.engine.Get([]byte(key))
	if err != nil {
		return nil, false, err
	}
	if found {
		return ref, true, nil
	}

	// Write to shard engine.
	if err := s.engine.Put(ctx, []segment.Entry{{
		Key:   []byte(key),
		Value: data,
	}}); err != nil {
		return nil, false, err
	}
	return ref, false, nil
}

// PutBlockBatch writes multiple blocks as one lower-layer engine batch.
func (s *BlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	if len(entries) == 0 {
		return nil
	}

	batch := make([]segment.Entry, 0, len(entries))
	for _, entry := range entries {
		key, err := encodeRef(entry.Ref)
		if err != nil {
			return err
		}
		batch = append(batch, segment.Entry{
			Key:       []byte(key),
			Value:     entry.Data,
			Tombstone: entry.Tombstone,
		})
	}
	return s.engine.Put(ctx, batch)
}

// PutBlockBackground writes a single block at background priority.
func (s *BlockStore) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if len(data) == 0 {
		return nil, false, block.ErrEmptyBlock
	}

	if opts == nil {
		opts = &block.PutOpts{}
	} else {
		opts = opts.CloneVT()
	}
	opts.HashType = opts.SelectHashType(s.hashType)

	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	if forceRef := opts.GetForceBlockRef(); !forceRef.GetEmpty() {
		if !ref.EqualsRef(forceRef) {
			return ref, false, block.ErrBlockRefMismatch
		}
	}

	key, err := encodeRef(ref)
	if err != nil {
		return nil, false, err
	}

	_, found, err := s.engine.Get([]byte(key))
	if err != nil {
		return nil, false, err
	}
	if found {
		return ref, true, nil
	}

	if err := s.engine.PutBackground(ctx, []segment.Entry{{
		Key:   []byte(key),
		Value: data,
	}}); err != nil {
		return nil, false, err
	}
	return ref, false, nil
}

// GetBlock gets a block by reference.
func (s *BlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if err := ref.Validate(false); err != nil {
		return nil, false, err
	}

	key, err := encodeRef(ref)
	if err != nil {
		return nil, false, err
	}

	return s.engine.Get([]byte(key))
}

// GetBlockExists checks if a block exists.
func (s *BlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	key, err := encodeRef(ref)
	if err != nil {
		return false, err
	}

	_, found, err := s.engine.Get([]byte(key))
	return found, err
}

// RmBlock deletes a block by writing a tombstone.
func (s *BlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	key, err := encodeRef(ref)
	if err != nil {
		return err
	}

	entry := segment.Entry{
		Key:       []byte(key),
		Tombstone: true,
	}
	return s.engine.Put(ctx, []segment.Entry{entry})
}

// StatBlock returns metadata about a block without reading its data.
func (s *BlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	key, err := encodeRef(ref)
	if err != nil {
		return nil, err
	}

	val, found, err := s.engine.Get([]byte(key))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, nil
	}

	return &block.BlockStat{Ref: ref, Size: int64(len(val))}, nil
}

// encodeRef encodes a BlockRef as a base58 string key.
func encodeRef(ref *block.BlockRef) (string, error) {
	rm, err := ref.MarshalKey()
	if err != nil {
		return "", err
	}
	return b58.FastBase58Encoding(rm), nil
}

// _ is a type assertion.
var _ block.StoreOps = (*BlockStore)(nil)

// _ is a type assertion.
var _ block.BatchPutStore = (*BlockStore)(nil)

// _ is a type assertion.
var _ block.BackgroundPutStore = (*BlockStore)(nil)
