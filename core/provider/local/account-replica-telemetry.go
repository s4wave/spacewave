package provider_local

import (
	"slices"
	"strings"

	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
)

// GetAccountTransferSnapshot aggregates real DEX payload traffic on the active
// account transport. A peer participating in several Spaces is counted once.
func (a *ProviderAccount) GetAccountTransferSnapshot() (dex_solicit.TransferSnapshot, []<-chan struct{}) {
	// Collect the transfer controllers and their change waiters.
	var controllers []*dex_solicit.Controller
	var waits []<-chan struct{}
	a.p2pSyncBcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
		waits = append(waits, getWait())
		if state := a.p2pSync; state != nil {
			state.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
				waits = append(waits, getWait())
				for _, ctrl := range state.exchanges {
					controllers = append(controllers, ctrl)
				}
			})
		}
	})

	// Aggregate each controller's snapshot, counting shared peers once.
	var result dex_solicit.TransferSnapshot
	peers := make(map[string]dex_solicit.PeerTransferSnapshot)
	for _, ctrl := range controllers {
		snapshot, wait := ctrl.GetTransferSnapshot()
		waits = append(waits, wait)
		result.UploadedBytes += snapshot.UploadedBytes
		result.DownloadedBytes += snapshot.DownloadedBytes
		if snapshot.LastActivity.After(result.LastActivity) {
			result.LastActivity = snapshot.LastActivity
		}
		for _, peer := range snapshot.Peers {
			combined := peers[peer.PeerID]
			combined.PeerID = peer.PeerID
			combined.Connected = combined.Connected || peer.Connected
			combined.UploadedBytes += peer.UploadedBytes
			combined.DownloadedBytes += peer.DownloadedBytes
			peers[peer.PeerID] = combined
		}
	}

	// Return the peers sorted by ID with the change waiters.
	for _, peer := range peers {
		result.Peers = append(result.Peers, peer)
	}
	slices.SortFunc(result.Peers, func(a, b dex_solicit.PeerTransferSnapshot) int { return strings.Compare(a.PeerID, b.PeerID) })
	return result, waits
}
