package block_store

import (
	"context"

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
	// StoreOps are the block store operations.
	block.StoreOps

	// id is the block store id
	id string
}

// NewStore constructs a store with an id and opts.
func NewStore(id string, ops block.StoreOps) Store {
	return &store{StoreOps: ops, id: id}
}

// GetID returns the ID of the block store.
func (s *store) GetID() string {
	return s.id
}

// PutBlockBatch forwards batched writes to the inner StoreOps when supported.
func (s *store) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	batcher, ok := s.StoreOps.(block.BatchPutStore)
	if !ok {
		for _, entry := range entries {
			if entry.Tombstone {
				if err := s.StoreOps.RmBlock(ctx, entry.Ref); err != nil {
					return err
				}
				continue
			}
			if _, _, err := s.StoreOps.PutBlock(ctx, entry.Data, &block.PutOpts{
				ForceBlockRef: entry.Ref.Clone(),
			}); err != nil {
				return err
			}
		}
		return nil
	}
	return batcher.PutBlockBatch(ctx, entries)
}

// PutBlockBackground forwards background writes to the inner StoreOps when supported.
func (s *store) PutBlockBackground(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	bg, ok := s.StoreOps.(block.BackgroundPutStore)
	if !ok {
		return s.StoreOps.PutBlock(ctx, data, opts)
	}
	return bg.PutBlockBackground(ctx, data, opts)
}

// BeginDeferFlush forwards to the inner StoreOps if it supports deferred flushing.
func (s *store) BeginDeferFlush() {
	if df, ok := s.StoreOps.(block.DeferFlushable); ok {
		df.BeginDeferFlush()
	}
}

// EndDeferFlush forwards to the inner StoreOps if it supports deferred flushing.
func (s *store) EndDeferFlush(ctx context.Context) error {
	if df, ok := s.StoreOps.(block.DeferFlushable); ok {
		return df.EndDeferFlush(ctx)
	}
	return nil
}

// _ is a type assertion
var (
	_ Store                    = ((*store)(nil))
	_ block.BatchPutStore      = ((*store)(nil))
	_ block.BackgroundPutStore = ((*store)(nil))
	_ block.DeferFlushable     = ((*store)(nil))
)
