package provider_spacewave

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/peer"
)

// authorizeAccountSession admits a remote peer listed among the account's live
// cloud sessions. Revoked sessions leave that list. The account state is
// cached; the cloud's account_changed push refreshes it.
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

// watchAccountSessions calls changed whenever the cached list of the
// account's cloud sessions changes.
func (a *ProviderAccount) watchAccountSessions(ctx context.Context, changed func()) error {
	var last []*api.AccountSessionInfo
	for {
		// Read the cached session list and the wait for its next change.
		var sessions []*api.AccountSessionInfo
		var waitCh <-chan struct{}
		a.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			sessions = a.state.sessions
			waitCh = getWaitCh()
		})

		// Report a change in the listed session peers.
		if !slices.EqualFunc(last, sessions, func(x, y *api.AccountSessionInfo) bool {
			return x.GetPeerId() == y.GetPeerId()
		}) {
			last = sessions
			changed()
		}

		// Wait for the next account state change.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}
