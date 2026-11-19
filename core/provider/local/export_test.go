package provider_local

// RetainsP2PPeer reports whether a keeps a link to the peer remote across P2P
// sync restarts.
func (a *ProviderAccount) RetainsP2PPeer(remote string) bool {
	var retained bool
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, retained = a.p2pPeerIDs[remote]
	})
	return retained
}

// ForgetP2PPeers drops the peers a retains across P2P sync restarts, as a
// process restart does, so the next retention starts from what a fresh
// account derives. Links already held on the running generation stay.
func (a *ProviderAccount) ForgetP2PPeers() {
	a.p2pSyncBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		a.p2pPeerIDs = nil
	})
}
