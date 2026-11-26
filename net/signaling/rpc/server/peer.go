package signaling_rpc_server

import signaling "github.com/s4wave/spacewave/net/signaling/rpc"

// serverPeerTracker tracks a peer on the server.
// Its methods run under Server.mtx.
type serverPeerTracker struct {
	// wait is a channel that is closed when below changes.
	wait chan struct{}
	// listening indicates the Listen RPC is attached.
	listening bool
	// listenNonce is the nonce of the current listen session.
	listenNonce uint64
	// wantPeers contains the string IDs of peers requesting a session with this peer.
	wantPeers map[string]struct{}
}

// beginListen attaches a listener, superseding an existing tracker's listener.
func (p *serverPeerTracker) beginListen(supersede bool) uint64 {
	// Wake the previous listener before attaching its replacement.
	if supersede {
		p.listenNonce++
		p.broadcast()
	}

	// Return the generation that fences this listener's cleanup.
	p.listening = true
	return p.listenNonce
}

// endListen detaches the listener only if its nonce is still current.
func (p *serverPeerTracker) endListen(nonce uint64) bool {
	// Leave a replacement listener attached.
	if p.listenNonce != nonce {
		return false
	}

	// Invalidate the listener and wake reconciliation before release.
	p.listenNonce++
	p.listening = false
	p.broadcast()
	return true
}

// nextListenChanges selects one want and clear relative to the sent membership.
// The returned wait channel observes subsequent changes to this snapshot.
func (p *serverPeerTracker) nextListenChanges(nonce uint64, sent map[string]struct{}) (want, clear string, wait <-chan struct{}, err error) {
	// Reject a listener superseded by another stream.
	if p.listenNonce != nonce {
		return "", "", nil, signaling.ErrUserpedListen
	}

	// Select the next desired peer the listener has not announced.
	for peerID := range p.wantPeers {
		if _, ok := sent[peerID]; !ok {
			want = peerID
			break
		}
	}

	// Select the next announced peer whose session request ended.
	for peerID := range sent {
		if _, ok := p.wantPeers[peerID]; !ok {
			clear = peerID
			break
		}
	}

	// Capture notification under the same lock as the membership snapshot.
	if p.wait == nil {
		p.wait = make(chan struct{})
	}
	return want, clear, p.wait, nil
}

// addWant registers a session request and wakes the listener when it is new.
func (p *serverPeerTracker) addWant(peerID string) {
	if _, ok := p.wantPeers[peerID]; !ok {
		p.wantPeers[peerID] = struct{}{}
		p.broadcast()
	}
}

// removeWant withdraws a session request and wakes the listener.
func (p *serverPeerTracker) removeWant(peerID string) {
	delete(p.wantPeers, peerID)
	p.broadcast()
}

// broadcast closes the wait channel if any.
func (p *serverPeerTracker) broadcast() {
	if p.wait != nil {
		close(p.wait)
		p.wait = nil
	}
}
