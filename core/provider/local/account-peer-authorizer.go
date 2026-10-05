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

// watchAccountSessions calls changed after each account settings change, since
// a change may add or revoke account sessions. It returns nil at once when the
// account has no settings.
func (a *ProviderAccount) watchAccountSessions(ctx context.Context, changed func()) error {
	// Follow the account settings.
	ref, err := a.lookupAccountSettingsRef(ctx)
	if err != nil || ref == nil {
		return err
	}
	watch, err := a.watchAccountSettings(ctx, ref)
	if err != nil {
		return err
	}
	defer watch.release()

	// Report each settings change.
	for {
		if _, err := watch.next(ctx); err != nil {
			return err
		}
		changed()
	}
}
