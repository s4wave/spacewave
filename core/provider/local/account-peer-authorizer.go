package provider_local

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// authorizeAccountSession admits a remote peer that is an active, unrevoked
// session of this account. The session transport applies it to inbound UDP
// links and to handlers that grant account authority.
func (a *ProviderAccount) authorizeAccountSession(ctx context.Context, remotePeer peer.ID) error {
	// Read the account's session list.
	settings, err := a.readAccountSettings(ctx)
	if err != nil {
		return err
	}

	// Admit the peer when its session is present and not revoked.
	member := settings.FindAccountSession(remotePeer.String())
	if member == nil || member.GetRevoked() {
		return errors.New("peer is not an active session of this account")
	}
	return nil
}
