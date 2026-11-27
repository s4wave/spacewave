package signaling_rpc_server

import "github.com/pkg/errors"

// sessionTracker tracks a session between two peers.
// Its methods run under Server.mtx.
type sessionTracker struct {
	// wait is a channel that is closed when the session tracker state
	// changes.
	wait chan struct{}
	// seqno is incremented when peerA or peerB changes.
	seqno uint64
	// peerA is the tracker for peer A.
	// nil until connected.
	peerA *sessionPeerTracker
	// peerB is the tracker for peer B.
	// nil until connected.
	peerB *sessionPeerTracker
}

// claimPeer replaces one endpoint and invalidates the previous generation.
func (t *sessionTracker) claimPeer(isPeerA bool, peer *sessionPeerTracker) {
	// Attach the replacement endpoint to its session slot.
	if isPeerA {
		t.peerA = peer
	} else {
		t.peerB = peer
	}

	// Pending messages and acknowledgments belong to the replaced generation.
	if _, remote := t.getCurrPeers(isPeerA); remote != nil {
		*remote = sessionPeerTracker{}
	}

	// Publish the new generation after both endpoint slots are consistent.
	t.seqno++
	t.broadcast()
}

// releasePeer detaches an endpoint only if it still occupies its session slot.
func (t *sessionTracker) releasePeer(isPeerA bool, peer *sessionPeerTracker) bool {
	// Fence cleanup from a stream superseded by another endpoint.
	local, remote := t.getCurrPeers(isPeerA)
	if local != peer {
		return false
	}

	// Detach this stream and drop remote control state from the closed generation.
	if isPeerA {
		t.peerA = nil
	} else {
		t.peerB = nil
	}
	if remote != nil {
		*remote = sessionPeerTracker{}
	}

	// Publish the generation change before the server releases unused trackers.
	t.seqno++
	t.broadcast()
	return true
}

// getWaitCh returns the wait channel.
func (t *sessionTracker) getWaitCh() <-chan struct{} {
	// Retain one channel until the next session change wakes its waiters.
	wait := t.wait
	if wait == nil {
		wait = make(chan struct{})
		t.wait = wait
	}
	return wait
}

// broadcast closes the wait channel if any.
func (t *sessionTracker) broadcast() {
	if t.wait != nil {
		close(t.wait)
		t.wait = nil
	}
}

// getCurrPeers returns the current peers for the session.
func (t *sessionTracker) getCurrPeers(srcIsPeerA bool) (*sessionPeerTracker, *sessionPeerTracker) {
	if srcIsPeerA {
		return t.peerA, t.peerB
	}
	return t.peerB, t.peerA
}

// checkSeqno checks if a seqno is equal to the current seqno and is a
// reasonable value.
func (t *sessionTracker) checkSeqno(seqno uint64) (bool, error) {
	// If the session sequence number is lower than the message session seqno, the peer is misbehaving.
	if t.seqno < seqno {
		return false, errors.Errorf("signaling: message session seqno is too high: %v > expected %v", seqno, t.seqno)
	}
	return t.seqno == seqno, nil
}
