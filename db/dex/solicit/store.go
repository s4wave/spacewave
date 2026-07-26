package dex_solicit

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
)

// Store is a read-only block store view owned by a solicitation Controller.
// Reads snapshot the controller's current peer sessions and fan out directly.
type Store struct {
	controller *Controller
}

// NewStore constructs a read-only block store view for a controller.
func NewStore(controller *Controller) *Store {
	return &Store{controller: controller}
}

// GetHashType returns the unset preferred hash type because the peer set may
// serve references using more than one hash type.
func (*Store) GetHashType() hash.HashType { return 0 }

// GetSupportedFeatures returns no writable or native batch features.
func (*Store) GetSupportedFeatures() block.StoreFeature { return 0 }

// BeginReadOperation opens a no-op read scope for the controller view.
func (s *Store) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock is unsupported because the DEX view is read-only.
func (*Store) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, block_store.ErrReadOnly
}

// PutBlockBatch is unsupported because the DEX view is read-only.
func (*Store) PutBlockBatch(context.Context, []*block.PutBatchEntry) error {
	return block_store.ErrReadOnly
}

// RmBlock is unsupported because the DEX view is read-only.
func (*Store) RmBlock(context.Context, *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Sync reports that the read-only view has no durability barrier.
func (*Store) Sync(context.Context) (bool, error) { return true, nil }

// GetBlock fans the request out to the controller's current peer sessions.
func (s *Store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found := peerBlockFanout{
		sessions: s.controller.snapshotSessions(),
		ref:      ref,
		hops:     s.controller.cc.GetMaxForwardHops(),
	}.run(ctx)
	return data, found, nil
}

// GetBlockExists checks whether any connected peer has the block.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	_, found, err := s.GetBlock(ctx, ref)
	return found, err
}

// GetBlockExistsBatch checks whether any connected peer has each block.
func (s *Store) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := s.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// StatBlock returns metadata for a block found on a connected peer.
func (s *Store) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	data, found, err := s.GetBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	return &block.BlockStat{Ref: ref, Size: int64(len(data))}, nil
}

var _ block.StoreOps = ((*Store)(nil))
