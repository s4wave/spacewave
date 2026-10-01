package provider_spacewave

import (
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	"github.com/s4wave/spacewave/core/provider"
	provider_migration "github.com/s4wave/spacewave/core/provider/migration"
)

var errAccountTransitionPending = errors.New("this Session is moving to its destination account")

// observeAccountTransition separates the server's new attachment from this
// account's cached data. A later transition may supersede an offline attachment.
func (a *ProviderAccount) observeAccountTransition(transition *provider.AccountTransition, accountID, peerID string) bool {
	// Reject transitions that do not apply to this account and Session peer.
	if transition == nil || transition.Validate() != nil || !slices.Contains(transition.GetSessionPeerIds(), peerID) {
		return false
	}

	// Ignore transitions sourced from a different provider endpoint.
	if transition.GetSourceEndpoint() != a.p.endpoint {
		return false
	}

	// Ignore re-observed transitions for this account.
	if accountID == a.accountID && transition.GetSource().GetProviderAccountId() != a.accountID {
		return false
	}

	// Validate destination transitions against this provider endpoint.
	if accountID != a.accountID && (transition.GetDestinationEndpoint() != a.p.endpoint || transition.GetDestination().GetProviderAccountId() != accountID) {
		return false
	}

	// Store the transition under the account broadcast lock.
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !a.state.transition.EqualVT(transition) {
			a.state.transition = transition.CloneVT()
			broadcast()
		}
	})
	return true
}

// watchAccountTransition follows cloud authorization under the unlocked Session
// lifetime. The initial read also covers a Session that was offline at commit.
func (s *Session) watchAccountTransition(ctx context.Context) error {
	// Load the owning ProviderAccount for this Session.
	a := s.tkr.a

	// Build a migration client and read the cloud account info.
	client, err := a.migrationClient(s.GetPrivKey())
	if err != nil {
		return err
	}
	info, err := client.GetAccountInfo(ctx)
	if err != nil {
		return err
	}

	// Apply the observed transition to this account.
	a.observeAccountTransition(info.GetTransition(), info.GetAccountId(), s.GetPeerId().String())
	for {
		// Wait for account transitions until the Session lifetime ends.
		var transition *provider.AccountTransition
		var changed <-chan struct{}
		// Snapshot the current transition and its change channel.
		a.accountBcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			transition = a.state.transition
			changed = getWait()
		})

		// Handle an active transition by attaching the Session to its destination.
		if transition != nil {
			s.pairingMu.Lock()
			engine := s.pairingEngine
			s.pairingMu.Unlock()
			// Wait for the pairing engine to settle the merge choice.
			if engine != nil {
				for {
					snapshot, changed := engine.Snapshot()
					if snapshot.Status == pairing.StatusBothConfirmed && snapshot.Choice.Merging() {
						// The pairing owner already committed this attachment.
						<-ctx.Done()
						return ctx.Err()
					}
					if snapshot.Status != pairing.StatusEnrolling || !snapshot.Choice.Merging() {
						break
					}
					select {
					case <-ctx.Done():
						return ctx.Err()
					case <-changed:
					}
				}
			}
			// Look up the destination provider and its account.
			p, releaseProvider, err := provider.ExLookupProvider(ctx, a.p.b, transition.GetDestination().GetProviderId(), false, nil)
			if err != nil {
				return err
			}
			defer releaseProvider.Release()
			if p == nil {
				return errors.New("destination provider is not configured")
			}
			account, releaseAccount, err := p.AccessProviderAccount(ctx, transition.GetDestination().GetProviderAccountId(), nil)
			if err != nil {
				return err
			}
			defer releaseAccount()
			// Attach this Session to the destination account.
			target, ok := account.(provider_migration.Account)
			if !ok {
				return errors.New("destination provider cannot accept this returning Session")
			}
			next, err := target.AttachMigratedSession(ctx, s, transition)
			if err != nil {
				return err
			}
			// Rebind the Session and hold until the lifetime ends.
			if err := provider_migration.RebindSession(ctx, s, next); err != nil {
				return err
			}
			<-ctx.Done()
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}
}
