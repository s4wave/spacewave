package provider_spacewave

import (
	"context"
	"time"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/accountstatus"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// accountStateCacheKey is the ObjectStore key for the account state cache.
const accountStateCacheKey = "account-state-cache"

// accountFetcherRetryOwner tracks transient retry delay for accountFetcher.
//
// The wake channel is captured with the account snapshot that produced the
// failed fetch. If account epoch or session-client readiness changes while a
// request is in flight, that channel closes and the remaining backoff is
// skipped so the fetcher can re-check current account state.
type accountFetcherRetryOwner struct {
	bo cbackoff.BackOff
}

// newAccountFetcherRetryOwner constructs a retry owner with the provider backoff.
func newAccountFetcherRetryOwner() *accountFetcherRetryOwner {
	return &accountFetcherRetryOwner{bo: providerBackoff.Construct()}
}

// Reset restarts the backoff after a successful fetch.
func (r *accountFetcherRetryOwner) Reset() {
	r.bo.Reset()
}

// WaitTransient waits the next backoff delay for err, returning early when
// wakeCh closes.
func (r *accountFetcherRetryOwner) WaitTransient(
	ctx context.Context,
	wakeCh <-chan struct{},
	err error,
) error {
	delay := nextProviderRetryDelay(r.bo, err)
	return waitAccountFetcherRetryDelay(ctx, wakeCh, delay)
}

// waitAccountFetcherRetryDelay waits for delay, wakeCh, or ctx, whichever comes
// first, and returns ctx's error only when ctx ends the wait.
func waitAccountFetcherRetryDelay(
	ctx context.Context,
	wakeCh <-chan struct{},
	delay time.Duration,
) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-wakeCh:
		return nil
	case <-timer.C:
		return nil
	}
}

// accountFetcher refreshes persisted state on startup and after account epoch
// changes, with one account state request per refresh. Cached state provides immediate reads but never replaces the first
// live refresh. One goroutine waits for changes through accountBcast.
func (a *ProviderAccount) accountFetcher(ctx context.Context) error {
	// Initialize account-fetcher logging and retry state.
	le := a.le.WithField("component", "account-fetcher")
	retry := newAccountFetcherRetryOwner()
	var prevKeypairs []*session.EntityKeypair
	for {
		// Read the current account epoch, client, and wake channel.
		var epoch, lastFetched uint64
		var cli *SessionClient
		var ch <-chan struct{}
		var bootstrapped bool
		a.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			// Snapshot the fetch inputs under the broadcast lock.
			epoch = a.state.epoch
			lastFetched = a.state.lastFetchedEpoch
			cli = a.sessionClient
			bootstrapped = a.state.accountBootstrapFetched
			ch = getWaitCh()
		})

		// Wait for a usable authenticated client when an epoch needs fetching.
		if !bootstrapped || epoch > lastFetched {
			if cli == nil || cli.SignedHTTPClient == nil || cli.peerID == "" || (cli.priv == nil && cli.sign == nil) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ch:
					continue
				}
			}

			// Fetch account state for the current epoch.
			state, err := cli.GetAccountState(ctx)
			if err != nil {
				if isNonRetryableCloudError(err) {
					if isUnauthCloudError(err) {
						if err := a.waitReauth(ctx, err); err != nil {
							return err
						}
						continue
					}
					le.WithError(err).Warn("permanent error fetching account state")
					return err
				}
				le.WithError(err).Warn("failed to fetch account state, will retry")
				if err := retry.WaitTransient(ctx, ch, err); err != nil {
					return err
				}
				continue
			}

			// Stop when the account moved to another attachment.
			if a.observeAccountTransition(state.GetTransition(), state.GetAccountId(), cli.peerID.String()) {
				<-ctx.Done()
				return ctx.Err()
			}

			// Apply the fetched state and reset transient retries.
			retry.Reset()

			le.WithFields(logrus.Fields{
				"epoch":         state.GetEpoch(),
				"keypair-count": state.GetKeypairCount(),
			}).Debug("fetched account state")

			keypairsChanged := !protobuf_go_lite.IsEqualVTSlice(prevKeypairs, state.GetKeypairs())
			prevKeypairs = state.GetKeypairs()

			// Publish fetched state and refresh dependent access state.
			a.applyFetchedAccountState(epoch, state)
			a.syncSharedObjectListAccess(state.GetSubscriptionStatus())
			a.refreshSelfRejoinSweepState()

			// Replace startup cache even when coverage changed without an epoch change.
			if !bootstrapped || uint64(state.GetEpoch()) > lastFetched {
				if err := a.writeAccountStateCache(ctx, state); err != nil {
					le.WithError(err).Warn("failed to write account state cache")
				}
			}

			// Rewrap the session envelope when account keypairs changed.
			if keypairsChanged && len(state.GetKeypairs()) > 0 {
				le.Debug("keypairs changed, rewrapping session envelope")
				if err := a.RewrapSessionEnvelope(ctx); err != nil {
					le.WithError(err).Warn("failed to rewrap session envelope after keypair change")
				}
			}
		}

		// Wait for the next epoch change or cancellation.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// waitReauth marks the account unauthenticated after err and
// waits until its status changes. It returns err when the account was deleted
// and nil when the account is usable again.
func (a *ProviderAccount) waitReauth(ctx context.Context, err error) error {
	// Publish the unauthenticated status once.
	var rejoinState *selfRejoinSweepState
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.state.status != provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
			a.state.status = accountstatus.Unauthenticated(a.state.info)
			rejoinState = a.buildSelfRejoinSweepStateLocked()
			broadcast()
		}
	})
	a.setSelfRejoinSweepState(rejoinState)

	// Wait for a status other than unauthenticated.
	for {
		var ch <-chan struct{}
		var status provider.ProviderAccountStatus
		a.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			status = a.state.status
			ch = getWaitCh()
		})
		if status == provider.ProviderAccountStatus_ProviderAccountStatus_DELETED {
			return err
		}
		if status != provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// applyFetchedAccountState stores freshly fetched account state, including the
// email and session lists it carries.
//
// If no newer invalidation arrived while the fetch was in flight, collapse the
// local trigger epoch back down to the fetched server epoch. This lets a
// bootstrap fetch at local epoch 1 with server epoch 0 settle to 0 so a later
// remote account_changed(epoch=1) still triggers a refetch.
func (a *ProviderAccount) applyFetchedAccountState(
	startEpoch uint64,
	state *api.AccountStateResponse,
) {
	// Store the fetched state under the account lock.
	var reconcileState *sessionPresentationReconcileState
	var rejoinState *selfRejoinSweepState
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Store the state with its email and session lists.
		a.state.info = state
		a.state.status = accountstatus.Loaded(state)
		a.state.accountBootstrapFetched = true
		a.state.cachedEmails = state.GetEmails()
		a.state.cachedEmailsValid = true
		a.state.sessions = state.GetSessions()
		a.state.sessionsValid = true
		a.state.infoFetching = false

		// Settle the epoch unless a newer invalidation arrived.
		fetchedEpoch := uint64(state.GetEpoch())
		if a.state.epoch == startEpoch {
			a.state.epoch = fetchedEpoch
		}
		if fetchedEpoch > a.state.lastFetchedEpoch {
			a.state.lastFetchedEpoch = fetchedEpoch
		}

		// Snapshot the routine states and wake watchers.
		reconcileState = a.buildSessionPresentationReconcileStateLocked()
		rejoinState = a.buildSelfRejoinSweepStateLocked()
		broadcast()
	})

	// Hand the new snapshots to the reconcile and rejoin routines.
	a.setSessionPresentationReconcileState(reconcileState)
	a.setSelfRejoinSweepState(rejoinState)
}

// writeAccountStateCache serializes AccountStateCache and writes it to ObjectStore.
func (a *ProviderAccount) writeAccountStateCache(ctx context.Context, state *api.AccountStateResponse) error {
	// Encode the state with its fetched epoch.
	cache := &api.AccountStateCache{
		State:        state,
		FetchedEpoch: state.GetEpoch(),
	}
	data, err := cache.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal account state cache")
	}
	err = kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			if err := tx.Set(ctx, []byte(accountStateCacheKey), data); err != nil {
				return errors.Wrap(err, "set account state cache")
			}
			return nil
		},
	)
	return errors.Wrap(err, "write account state cache")
}

// loadAccountStateCache reads AccountStateCache from ObjectStore.
// Returns nil if the cache does not exist.
func (a *ProviderAccount) loadAccountStateCache(ctx context.Context) (*api.AccountStateCache, error) {
	var cache *api.AccountStateCache
	err := kvtx.RunTransaction(ctx, false,
		func(ctx context.Context) (kvtx.Tx, error) {
			return a.objStore.NewTransaction(ctx, false)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			// Read the stored cache entry, if any.
			data, found, err := tx.Get(ctx, []byte(accountStateCacheKey))
			if err != nil {
				return errors.Wrap(err, "get account state cache")
			}
			if !found {
				cache = nil
				return nil
			}
			next := &api.AccountStateCache{}
			if err := next.UnmarshalVT(data); err != nil {
				return errors.Wrap(err, "unmarshal account state cache")
			}
			cache = next
			return nil
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "open read transaction")
	}
	return cache, nil
}
