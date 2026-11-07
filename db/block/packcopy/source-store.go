package packcopy

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// errSourceStoreWrite rejects a write to a SourceStore.
var errSourceStoreWrite = errors.New("source store is read-only")

// SourceStore reads blocks from sources in order, for code that reads a
// graph through a block store, such as a World snapshot. It serves the bytes
// alone; writes and the store's other operations are refused.
type SourceStore struct {
	block.StoreOps
	// sources serve the blocks, tried in order.
	sources []Source
}

// NewSourceStore returns a read-only store over sources.
func NewSourceStore(sources []Source) *SourceStore {
	return &SourceStore{sources: sources}
}

// GetHashType returns the default hash type.
func (s *SourceStore) GetHashType() hash.HashType { return 0 }

// GetSupportedFeatures reports no native features.
func (s *SourceStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation returns the store itself.
func (s *SourceStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// GetStoredBlock returns the block from the first source that holds it with
// bytes matching ref.
func (s *SourceStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	for _, src := range s.sources {
		stored, err := src.GetStoredBlock(ctx, ref)
		if ctx.Err() != nil {
			return nil, context.Cause(ctx)
		}
		if err != nil || stored == nil {
			continue
		}
		if ref.VerifyData(stored.Data, false) == nil {
			return stored, nil
		}
	}
	return nil, nil
}

// GetBlock returns the block's bytes.
func (s *SourceStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

// GetBlockExists reports whether a source holds the block.
func (s *SourceStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

// GetBlockExistsBatch reports whether a source holds each block.
func (s *SourceStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		var err error
		if out[i], err = s.GetBlockExists(ctx, ref); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// PutBlock refuses the write.
func (s *SourceStore) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, errSourceStoreWrite
}

// PutBlockBatch refuses the write.
func (s *SourceStore) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, errSourceStoreWrite
}

// RmBlock refuses the write.
func (s *SourceStore) RmBlock(context.Context, *block.BlockRef) error {
	return errSourceStoreWrite
}
