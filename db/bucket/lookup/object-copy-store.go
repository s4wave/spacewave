package bucket_lookup

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// objectCopyStore resolves logical existence accounting once per bounded copy
// batch. StoreOps retains the destination's ownership and durability behavior.
type objectCopyStore struct {
	// inner owns destination storage and durability.
	inner block.StoreOps
	// account reports the batch before its write, including failed write attempts.
	account func([]bool) error
}

// PutBlockBatch checks the batch together, then writes even existing payloads
// because storage presence does not establish destination bucket ownership.
func (s *objectCopyStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	refs := make([]*block.BlockRef, len(entries))
	for i, entry := range entries {
		refs[i] = entry.Ref
	}
	existing, err := s.inner.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return err
	}
	if len(existing) != len(entries) {
		return errors.Errorf("copy existence batch returned %d results for %d blocks", len(existing), len(entries))
	}
	if err := s.account(existing); err != nil {
		return err
	}
	return s.inner.PutBlockBatch(ctx, entries)
}

// GetHashType returns the destination hash type.
func (s *objectCopyStore) GetHashType() hash.HashType {
	return s.inner.GetHashType()
}

// GetSupportedFeatures returns the destination storage features.
func (s *objectCopyStore) GetSupportedFeatures() block.StoreFeature {
	return s.inner.GetSupportedFeatures()
}

// BeginReadOperation opens a destination read scope.
func (s *objectCopyStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	return s.inner.BeginReadOperation(ctx)
}

// PutBlock forwards individual writes without batch accounting.
func (s *objectCopyStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return s.inner.PutBlock(ctx, data, opts)
}

// GetBlock reads a destination block.
func (s *objectCopyStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return s.inner.GetBlock(ctx, ref)
}

// GetStoredBlock forwards to the inner store.
func (s *objectCopyStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return s.inner.GetStoredBlock(ctx, ref)
}

// GetBlockExists checks destination presence.
func (s *objectCopyStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.inner.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks destination presence for each reference.
func (s *objectCopyStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return s.inner.GetBlockExistsBatch(ctx, refs)
}

// RmBlock removes a destination block.
func (s *objectCopyStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return s.inner.RmBlock(ctx, ref)
}

// StatBlock reads destination metadata.
func (s *objectCopyStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return s.inner.StatBlock(ctx, ref)
}

// Sync fences all destination writes.
func (s *objectCopyStore) Sync(ctx context.Context) (bool, error) {
	return s.inner.Sync(ctx)
}

// _ verifies the complete storage forwarding contract.
var _ block.StoreOps = (*objectCopyStore)(nil)
