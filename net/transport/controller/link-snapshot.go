package transport_controller

import "github.com/s4wave/spacewave/net/peer"

// LinkSnapshot describes a live link held by a transport controller.
type LinkSnapshot struct {
	LinkID            uint64
	TransportID       uint64
	RemoteTransportID uint64
	LocalPeerID       peer.ID
	RemotePeerID      peer.ID
}

// GetLinkSnapshotsWithWait returns live link snapshots and a wait channel that
// closes when the link set changes.
func (c *Controller) GetLinkSnapshotsWithWait() ([]LinkSnapshot, <-chan struct{}) {
	var links []LinkSnapshot
	var waitCh <-chan struct{}
	c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		waitCh = getWaitCh()
		links = make([]LinkSnapshot, 0, len(c.links))
		for _, lnk := range c.links {
			links = append(links, LinkSnapshot{
				LinkID:            lnk.lnk.GetUUID(),
				TransportID:       lnk.lnk.GetTransportUUID(),
				RemoteTransportID: lnk.lnk.GetRemoteTransportUUID(),
				LocalPeerID:       lnk.lnk.GetLocalPeer(),
				RemotePeerID:      lnk.lnk.GetRemotePeer(),
			})
		}
	})
	return links, waitCh
}
