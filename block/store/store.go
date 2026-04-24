package block_store

import (
	"context"

	block_hash "github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/sirupsen/logrus"
)

// Store is a block store with an ID and read/write functions.
type Store interface {
	// GetID returns the ID of the block store.
	GetID() string

	// StoreOps are the block store operations.
	block.StoreOps
}

// Constructor constructs a block store with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
) (Store, error)

// store wraps StoreOps with an ID.
type store struct {
	// ops are the block store operations.
	ops block.StoreOps

	// id is the block store id
	id string
}

// NewStore constructs a store with an id and opts.
func NewStore(id string, ops block.StoreOps) Store {
	return &store{ops: ops, id: id}
}

// GetID returns the ID of the block store.
func (s *store) GetID() string {
	return s.id
}

// GetHashType returns the preferred hash type for the store.
func (s *store) GetHashType() block_hash.HashType {
	return s.ops.GetHashType()
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (s *store) GetSupportedFeatures() block.StoreFeature {
	return s.ops.GetSupportedFeatures()
}

// PutBlock forwards to the inner StoreOps.
func (s *store) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return s.ops.PutBlock(ctx, data, opts)
}

// PutBlockBatch forwards batched writes to the inner StoreOps.
func (s *store) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return s.ops.PutBlockBatch(ctx, entries)
}

// PutBlockBackground forwards background writes to the inner StoreOps.
func (s *store) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return s.ops.PutBlockBackground(ctx, data, opts)
}

// GetBlock forwards to the inner StoreOps.
func (s *store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return s.ops.GetBlock(ctx, ref)
}

// GetBlockExists forwards to the inner StoreOps.
func (s *store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.ops.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch forwards batched existence probes to the inner StoreOps.
func (s *store) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return s.ops.GetBlockExistsBatch(ctx, refs)
}

// RmBlock forwards to the inner StoreOps.
func (s *store) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return s.ops.RmBlock(ctx, ref)
}

// StatBlock forwards to the inner StoreOps.
func (s *store) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return s.ops.StatBlock(ctx, ref)
}

// Flush forwards to the inner StoreOps.
func (s *store) Flush(ctx context.Context) error {
	return s.ops.Flush(ctx)
}

// BeginDeferFlush forwards to the inner StoreOps.
func (s *store) BeginDeferFlush() {
	s.ops.BeginDeferFlush()
}

// EndDeferFlush forwards to the inner StoreOps.
func (s *store) EndDeferFlush(ctx context.Context) error {
	return s.ops.EndDeferFlush(ctx)
}

// _ is a type assertion
var (
	_ Store = ((*store)(nil))
)
