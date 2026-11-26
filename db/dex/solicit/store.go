package dex_solicit

import (
	"context"
	"fmt"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
)

// linkedAnswersKey is the context key of WithLinkedAnswers.
type linkedAnswersKey struct{}

// WithLinkedAnswers returns ctx under which a Store's GetStoredBlock lets each
// peer that accept admits answer linked: a linked file on that peer's Device
// is its only copy there, and the Store returns the block's refs, marked
// Linked, with no data. Only a copy that may leave such a block on that
// peer's Device sets it, and accept admits only peers trusted to keep the
// block, since a linked answer makes the copy skip it. GetBlock and existence
// checks always ask for the data.
func WithLinkedAnswers(ctx context.Context, accept func(peer.ID) bool) context.Context {
	return context.WithValue(ctx, linkedAnswersKey{}, accept)
}

// linkedAnswers returns the accept of WithLinkedAnswers, or nil.
func linkedAnswers(ctx context.Context) func(peer.ID) bool {
	accept, _ := ctx.Value(linkedAnswersKey{}).(func(peer.ID) bool)
	return accept
}

// Store is a read-only block store view owned by a solicitation Controller.
// Reads wait for the solicitation to settle and for at least one peer session,
// then fan out to the controller's peer sessions. A read under a context from
// block.WithoutPeerWait does not wait for a peer to connect: with no peer
// connected it records the block and reports block.ErrUnavailable, unless a
// peer's session ended within sessionReplaceWait, when it waits that long for
// the session to be restored. Existence checks do not wait for a
// peer: with none connected they report the block missing.
type Store struct {
	controller *Controller
}

// NewStore constructs a read-only block store view for a controller.
func NewStore(controller *Controller) *Store {
	return &Store{controller: controller}
}

// WithReadProgress returns ctx carrying a new read's progress, and a func
// returning the bytes delivered toward the read so far: what its peer
// sessions read while one of its requests was pending there, answers queued
// ahead of its own included. The count stops once a peer answers the read
// with a failure. A reader waiting on one block can thus tell a slow link,
// whose answers queue behind each other in the peer's session, from a peer
// that sends nothing or refuses.
func (*Store) WithReadProgress(ctx context.Context) (context.Context, func() uint64) {
	p := &readProgress{}
	return context.WithValue(ctx, readProgressKey{}, p), p.bytes
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
	found, err := s.fetch(ctx, ref, true, nil)
	if found == nil || err != nil {
		return nil, false, err
	}
	return found.GetData(), true, nil
}

// GetStoredBlock fans the request out to the controller's peer sessions.
// RefsKnown is unset when the answering peer held the block without its refs.
// Under WithLinkedAnswers, a linked answer returns the refs, marked Linked,
// with no data.
func (s *Store) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	found, err := s.fetch(ctx, ref, true, linkedAnswers(ctx))
	if found == nil || err != nil {
		return nil, err
	}
	if found.GetLinked() {
		return &block.StoredBlock{Refs: found.GetRefs(), RefsSource: block.RefsListed, Linked: true}, nil
	}
	return &block.StoredBlock{
		Data:       found.GetData(),
		Refs:       found.GetRefs(),
		RefsSource: block.ListedIf(found.GetRefsKnown()),
	}, nil
}

// fetch requests a block from the settled peer sessions, first waiting for a
// peer session when wantPeer is set and ctx allows waiting. Returns nil when
// every peer answered that it does not have the block, and an error when the
// exchange failed or no peer was there to ask, which wraps ErrNoPeerSession.
// linked admits the peers that may answer linked (peerBlockFanout).
func (s *Store) fetch(ctx context.Context, ref *block.BlockRef, wantPeer bool, linked func(peer.ID) bool) (*DexMessage, error) {
	// Settle the peer sessions, waiting for one only when ctx allows it.
	peerWait := block.GetPeerWait(ctx)
	skipWait := wantPeer && peerWait != nil
	sessions, err := s.controller.waitSessions(ctx, nil, wantPeer && !skipWait)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoPeerSession, err)
	}
	if skipWait && len(sessions) == 0 {
		// A peer whose session just ended is expected back within seconds,
		// so wait for its recovery rather than report the block unavailable.
		sessions, err = s.controller.waitRecovery(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrNoPeerSession, err)
		}
	}
	if skipWait && len(sessions) == 0 {
		peerWait.Skip(ref)
		return nil, fmt.Errorf("%w: %w to ask for block %s", block.ErrUnavailable, ErrNoPeerSession, ref.MarshalString())
	}

	// Ask the settled peer sessions for the block.
	f := peerBlockFanout{sessions: sessions, ref: ref, hops: s.controller.cc.GetMaxForwardHops(), linked: linked}
	found, _, err := s.controller.askPeers(ctx, f, nil)
	if err != nil {
		if p := getReadProgress(ctx); p != nil && peerRefused(err) {
			p.refuse()
		}
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

// HasPeerSession reports whether a peer session is open now, so a read that
// must not wait for a peer can skip asking when none is.
func (s *Store) HasPeerSession() bool {
	return s.controller.hasOpenSession()
}

// GetBlockExists checks whether any connected peer has the block. It does not
// wait for a peer to connect.
func (s *Store) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	found, err := s.fetch(ctx, ref, false, nil)
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
