package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// authorizeAccountSession admits a remote peer listed among the account's live
// cloud sessions. Revoked sessions leave that list. The account state is
// cached, so a revocation applies once the cache refreshes.
func (a *ProviderAccount) authorizeAccountSession(ctx context.Context, remotePeer peer.ID) error {
	// Read the account's session list.
	state, err := a.GetAccountState(ctx)
	if err != nil {
		return err
	}

	// Admit the peer when it is listed.
	remote := remotePeer.String()
	for _, session := range state.GetSessions() {
		if session.GetPeerId() == remote {
			return nil
		}
	}
	return errors.New("peer is not an active session of this account")
}
