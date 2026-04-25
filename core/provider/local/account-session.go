package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/net/peer"
)

// GetMountedSessionPeerID returns the peer ID of a mounted local session.
func (a *ProviderAccount) GetMountedSessionPeerID(ctx context.Context) peer.ID {
	entries := a.sessions.GetKeysWithData()
	for _, entry := range entries {
		prom, _ := entry.Data.sessionProm.GetPromise()
		if prom == nil {
			continue
		}
		sess, err := prom.Await(ctx)
		if err != nil || sess == nil {
			continue
		}
		return sess.GetPeerId()
	}
	return ""
}
