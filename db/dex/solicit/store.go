package dex_solicit

import (
	"context"
	"fmt"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
)

// Store is a read-only block store view owned by a solicitation Controller.
// Reads wait for the solicitation to settle and for at least one peer session,
// then fan out to the controller's peer sessions. A read under a context from
// block.WithoutPeerWait does not wait: with no peer connected it records the
// block and reports block.ErrUnavailable. Existence checks do not wait for a
// peer: with none connected they report the block missing.
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
func (*Store) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, block_store.ErrReadOnly
}

// RmBlock is unsupported because the DEX view is read-only.
func (*Store) RmBlock(context.Context, *block.BlockRef) error {
	return block_store.ErrReadOnly
}

// Sync reports that the read-only view has no durability barrier.
func (*Store) Sync(context.Context) (bool, error) { return true, nil }

// GetBlock fans the request out to the controller's peer sessions.
func (s *Store) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	found, err := s.fetch(ctx, ref, true)
	if found == nil || err != nil {
		return nil, false, err
	}
	return found.GetData(), true, nil
}

// GetStoredBlock fans the request out to the controller's peer sessions.
// RefsKnown is unset when the answering peer held the block without its refs.
func (s *Store) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	found, err := s.fetch(ctx, ref, true)
	if found == nil || err != nil {
		return nil, err
	}
	return &block.StoredBlock{
		Data:      found.GetData(),
		Refs:      found.GetRefs(),
		RefsKnown: found.GetRefsKnown(),
	}, nil
}

// fetch requests a block from the settled peer sessions, first waiting for a
// peer session when wantPeer is set and ctx allows waiting. Returns nil when
// every peer answered that it does not have the block, and an error when the
// exchange failed or no peer was there to ask.
func (s *Store) fetch(ctx context.Context, ref *block.BlockRef, wantPeer bool) (*DexMessage, error) {
	// Settle the peer sessions, waiting for one only when ctx allows it.
	peerWait := block.GetPeerWait(ctx)
	skipWait := wantPeer && peerWait != nil
	sessions, err := s.controller.waitSessions(ctx, nil, wantPeer && !skipWait)
	if err != nil {
		return nil, err
	}
	if skipWait && len(sessions) == 0 {
		peerWait.Skip(ref)
		return nil, fmt.Errorf("%w: no peer session to ask for block %s", block.ErrUnavailable, ref.MarshalString())
	}

	// Ask the settled peer sessions for the block.
	found, err := peerBlockFanout{
		sessions: sessions,
		ref:      ref,
		hops:     s.controller.cc.GetMaxForwardHops(),
	}.run(ctx)
	if err != nil {
		return nil, err
	}

	// Log a miss with the number of peers asked.
	if found == nil {
		s.controller.le.
			WithField("session-count", len(sessions)).
			WithField("ref", ref.String()).
			Debug("dex block unavailable")
	}
	return found, nil
}

// GetBlockExists checks whether any connected peer has the block. It does not
// wait for a peer to connect.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	found, err := s.fetch(ctx, ref, false)
	return found != nil, err
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

// _ is a type assertion
var _ block.StoreOps = (*Store)(nil)
