//go:build js

package blockshard

import (
	"context"
	"runtime/trace"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/js/opfs/segment"
	"github.com/s4wave/spacewave/net/hash"
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
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/block-store/put-block-batch")
	defer task.End()

	if len(entries) == 0 {
		return nil
	}

	_, encodeTask := trace.NewTask(ctx, "hydra/opfs-blockshard/block-store/put-block-batch/encode-entries")
	batch := make([]segment.Entry, 0, len(entries))
	for _, entry := range entries {
		key, err := encodeRef(entry.Ref)
		if err != nil {
			encodeTask.End()
			return err
		}
		batch = append(batch, segment.Entry{
			Key:       []byte(key),
			Value:     entry.Data,
			Tombstone: entry.Tombstone,
		})
	}
	encodeTask.End()

	putCtx, putTask := trace.NewTask(ctx, "hydra/opfs-blockshard/block-store/put-block-batch/engine-put")
	err := s.engine.Put(putCtx, batch)
	putTask.End()
	return err
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
	ctx, task := trace.NewTask(ctx, "hydra/opfs-blockshard/block-store/get-block")
	defer task.End()

	if err := ref.Validate(false); err != nil {
		return nil, false, err
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-blockshard/block-store/get-block/encode-ref")
	key, err := encodeRef(ref)
	subtask.End()
	if err != nil {
		return nil, false, err
	}

	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-blockshard/block-store/get-block/engine-get")
	data, found, err := s.engine.GetContext(taskCtx, []byte(key))
	subtask.End()
	return data, found, err
}

// GetBlockExists checks if a block exists.
func (s *BlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	key, err := encodeRef(ref)
	if err != nil {
		return false, err
	}

	return s.engine.GetExists([]byte(key))
}

// GetBlockExistsBatch checks whether a batch of block refs exists.
func (s *BlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	keys := make([][]byte, len(refs))
	for i, ref := range refs {
		if ref == nil || ref.GetEmpty() {
			continue
		}
		key, err := encodeRef(ref)
		if err != nil {
			return nil, err
		}
		keys[i] = []byte(key)
	}
	return s.engine.GetExistsBatch(ctx, keys)
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
var _ block.BatchExistsStore = (*BlockStore)(nil)

// _ is a type assertion.
var _ block.BatchPutStore = (*BlockStore)(nil)

// _ is a type assertion.
var _ block.BackgroundPutStore = (*BlockStore)(nil)
