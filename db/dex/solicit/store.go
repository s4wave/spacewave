package dex_solicit

import (
	"context"
	"fmt"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/errgroup"
)

// Store is a read-only block store view owned by a solicitation Controller.
// Reads wait for the solicitation to settle and for at least one peer session,
// then fan out to the controller's peer sessions. A read under a context from
// block.WithoutPeerWait does not wait for a peer to connect: with no peer
// connected it records the block and reports block.ErrUnavailable, unless a
// peer's session ended within sessionReplaceWait, when it waits that long for
// the session to be restored. Existence checks do not wait for a peer: with
// none connected they report the block missing, or block.ErrUnavailable under
// block.WithoutPeerWait.
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
	// Settle the peer sessions, then ask them for the block.
	sessions, err := s.settle(ctx, wantPeer, ref)
	if err != nil {
		return nil, err
	}
	return s.ask(ctx, sessions, ref)
}

// settle returns the settled peer sessions, first waiting for one when
// wantPeer is set and ctx allows waiting. Under block.WithoutPeerWait with no
// session, after waiting out a recovering one, it records refs and reports
// block.ErrUnavailable.
func (s *Store) settle(ctx context.Context, wantPeer bool, refs ...*block.BlockRef) ([]*peerSession, error) {
	// Settle the peer sessions, waiting for one only when ctx allows it.
	peerWait := block.GetPeerWait(ctx)
	skipWait := wantPeer && peerWait != nil
	sessions, err := s.controller.waitSessions(ctx, nil, wantPeer && !skipWait)
	if err != nil || !skipWait || len(sessions) != 0 {
		return sessions, err
	}

	// A peer whose session just ended is expected back within seconds, so
	// wait for its recovery rather than report the blocks unavailable.
	sessions, err = s.controller.waitRecovery(ctx)
	if err != nil || len(sessions) != 0 {
		return sessions, err
	}
	for _, ref := range refs {
		peerWait.Skip(ref)
	}
	return nil, fmt.Errorf("%w: no peer session to ask for %d blocks", block.ErrUnavailable, len(refs))
}

// ask requests a block from the settled peer sessions. Returns nil when every
// peer answered that it does not have the block.
func (s *Store) ask(ctx context.Context, sessions []*peerSession, ref *block.BlockRef) (*DexMessage, error) {
	// Ask the settled peer sessions for the block.
	found, err := s.controller.askPeers(ctx, sessions, ref, s.controller.cc.GetMaxForwardHops(), nil)
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

// GetBlockExists checks whether any connected peer has the block, as
// GetBlockExistsBatch does for one ref.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	found, err := s.GetBlockExistsBatch(ctx, []*block.BlockRef{ref})
	if err != nil {
		return false, err
	}
	return found[0], nil
}

// existsBatchAsks bounds the blocks one existence batch asks the peers about
// at once.
const existsBatchAsks = 16

// GetBlockExistsBatch checks whether any connected peer has each block. It
// settles the sessions once for the batch, then asks about up to
// existsBatchAsks blocks at once, so a batch waits about as long as its
// slowest ask rather than the sum of them. It does not wait for a peer to
// connect: with no session it answers false.
//
// Under block.WithoutPeerWait a false is a proof of absence to its caller, and
// the connected peers are not known to be every device that may hold the
// block: a device of the Space that is offline may. So there a block no
// connected peer holds, or a batch with no session to ask, reports
// block.ErrUnavailable, with the answers found so far.
func (s *Store) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Settle the sessions once.
	peerWait := block.GetPeerWait(ctx)
	sessions, err := s.settle(ctx, peerWait != nil, refs...)
	if err != nil || len(refs) == 0 {
		return make([]bool, len(refs)), err
	}

	// Ask about every block, a bounded number at once.
	out := make([]bool, len(refs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(existsBatchAsks)
	for i, ref := range refs {
		g.Go(func() error {
			found, err := s.ask(gctx, sessions, ref)
			out[i] = found != nil
			return err
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	// Report a block no connected peer holds as unknown to a caller that
	// reads false as absent.
	if missed := slices.Index(out, false); peerWait != nil && missed != -1 {
		peerWait.Skip(refs[missed])
		return out, fmt.Errorf("%w: no connected peer holds the block, and an offline one may", block.ErrUnavailable)
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
