package block_store_vlogger

import (
	"context"
	"time"

	"github.com/aperturerobotics/bifrost/hash"
	"github.com/aperturerobotics/hydra/block"
	"github.com/sirupsen/logrus"
)

// VLoggerStore implements a verbose logger wrapping a block store.
type VLoggerStore struct {
	le *logrus.Entry
	st block.Store
}

// NewVLoggerStore constructs a new verbose logger wrapper for a block store.
func NewVLoggerStore(le *logrus.Entry, st block.Store) *VLoggerStore {
	return &VLoggerStore{le: le, st: st}
}

// GetHashType returns the preferred hash type for the store.
// This should return as fast as possible (called frequently).
// If 0 is returned, uses a default defined by Hydra.
func (s *VLoggerStore) GetHashType() hash.HashType {
	return s.st.GetHashType()
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
// The second return value can optionally indicate if the block already existed.
// If the hash type is unset, use the type from GetHashType().
func (s *VLoggerStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (ref *block.BlockRef, existed bool, err error) {
	t1 := time.Now()
	defer func() {
		s.le.Debugf(
			"PutBlock(len(%d)) => dur(%v) ref(%v) existed(%v) err(%v)",
			len(data),
			time.Since(t1).String(),
			ref.MarshalString(),
			existed,
			err,
		)
	}()
	return s.st.PutBlock(ctx, data, opts)
}

// GetBlock gets a block with the given reference.
// The ref should not be modified or retained by GetBlock.
// Returns data, found, error.
// Returns nil, false, nil if not found.
// Note: the block may not be in the specified bucket.
func (s *VLoggerStore) GetBlock(ctx context.Context, ref *block.BlockRef) (data []byte, found bool, err error) {
	t1 := time.Now()
	defer func() {
		s.le.Debugf(
			"GetBlock(%v) => dur(%v) data(%d) found(%v) err(%v)",
			ref.MarshalString(),
			time.Since(t1).String(),
			len(data),
			found,
			err,
		)
	}()
	return s.st.GetBlock(ctx, ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (s *VLoggerStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (found bool, err error) {
	t1 := time.Now()
	defer func() {
		s.le.Debugf(
			"GetBlockExists(%v) => dur(%v) found(%v) err(%v)",
			ref.MarshalString(),
			time.Since(t1).String(),
			found,
			err,
		)
	}()
	return s.st.GetBlockExists(ctx, ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (s *VLoggerStore) RmBlock(ctx context.Context, ref *block.BlockRef) (err error) {
	t1 := time.Now()
	defer func() {
		s.le.Debugf(
			"RmBlock(%v) => dur(%v) err(%v)",
			ref.MarshalString(),
			time.Since(t1).String(),
			err,
		)
	}()
	return s.st.RmBlock(ctx, ref)
}

// _ is a type assertion
var _ block.Store = ((*VLoggerStore)(nil))
