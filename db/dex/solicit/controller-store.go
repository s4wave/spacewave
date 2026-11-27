package dex_solicit

import (
	"context"
	"fmt"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
)

// fetchBlock requests a block from the settled peer sessions, first waiting for a
// peer session when wantPeer is set and ctx allows waiting. Returns nil when
// every peer answered that it does not have the block, and an error when the
// exchange failed or no peer was there to ask, which wraps ErrNoPeerSession.
// linked admits the peers that may answer linked (peerBlockFanout).
func (c *Controller) fetchBlock(ctx context.Context, ref *block.BlockRef, wantPeer bool, linked func(peer.ID) bool) (*DexMessage, error) {
	// Settle the peer sessions, waiting for one only when ctx allows it.
	peerWait := block.GetPeerWait(ctx)
	skipWait := wantPeer && peerWait != nil
	sessions, err := c.waitSessions(ctx, nil, wantPeer && !skipWait)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNoPeerSession, err)
	}
	if skipWait && len(sessions) == 0 {
		// A peer whose session just ended is expected back within seconds,
		// so wait for its recovery rather than report the block unavailable.
		sessions, err = c.waitRecovery(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrNoPeerSession, err)
		}
	}
	if skipWait && len(sessions) == 0 {
		peerWait.Skip(ref)
		return nil, fmt.Errorf("%w: %w to ask for block %s", block.ErrUnavailable, ErrNoPeerSession, ref.MarshalString())
	}

	// Ask the settled peer sessions for the block.
	f := peerBlockFanout{sessions: sessions, ref: ref, hops: c.cc.GetMaxForwardHops(), linked: linked}
	found, _, err := c.askPeers(ctx, f, nil)
	if err != nil {
		if p := getReadProgress(ctx); p != nil && peerRefused(err) {
			p.refuse()
		}
		return nil, err
	}

	// Log a miss with the number of peers asked.
	if found == nil {
		c.le.
			WithField("session-count", len(sessions)).
			WithField("ref", ref.String()).
			Debug("dex block unavailable")
	}
	return found, nil
}
