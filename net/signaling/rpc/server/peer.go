package signaling_rpc_server

// serverPeerTracker tracks a peer on the server.
type serverPeerTracker struct {
	// wait is a channel that is closed when below changes.
	wait chan struct{}
	// listening indicates the listen rpc is attached
	listening bool
	// listenNonce is the nonce of the current listen session
	listenNonce uint64
	// wantPeers is the list of peers that want a session w/ this peer.
	// peer ids encoded as string
	wantPeers map[string]struct{}
}

// getWaitCh returns the wait channel.
func (p *serverPeerTracker) getWaitCh() <-chan struct{} {
	wait := p.wait
	if wait == nil {
		wait = make(chan struct{})
		p.wait = wait
	}
	return wait
}

// broadcast closes the wait channel if any
func (p *serverPeerTracker) broadcast() {
	if p.wait != nil {
		close(p.wait)
		p.wait = nil
	}
}
