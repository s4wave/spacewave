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

// accountFetcherRetryOwner owns transient retry delay for accountFetcher.
//
// The wake channel is captured with the account snapshot that produced the
// failed fetch. If account epoch or session-client readiness changes while a
// request is in flight, that channel closes and the retry owner skips the
// remaining backoff so the fetcher can re-check current account state.
type accountFetcherRetryOwner struct {
	bo cbackoff.BackOff
}

func newAccountFetcherRetryOwner() *accountFetcherRetryOwner {
	return &accountFetcherRetryOwner{bo: providerBackoff.Construct()}
}

func (r *accountFetcherRetryOwner) Reset() {
	r.bo.Reset()
}

func (r *accountFetcherRetryOwner) WaitTransient(
	ctx context.Context,
	wakeCh <-chan struct{},
	err error,
) error {
	delay := nextProviderRetryDelay(r.bo, err)
	return waitAccountFetcherRetryDelay(ctx, wakeCh, delay)
}

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

// accountFetcher runs a loop that fetches account state from the cloud when the
// epoch advances past the last fetched epoch. Single goroutine, triggered by
// epoch changes via accountBcast.
func (a *ProviderAccount) accountFetcher(ctx context.Context) error {
	le := a.le.WithField("component", "account-fetcher")
	retry := newAccountFetcherRetryOwner()
	var prevKeypairs []*session.EntityKeypair
	for {
		var epoch, lastFetched uint64
		var cli *SessionClient
		var ch <-chan struct{}
		a.accountBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			epoch = a.state.epoch
			lastFetched = a.state.lastFetchedEpoch
			cli = a.sessionClient
			ch = getWaitCh()
		})

		if epoch > lastFetched {
			if cli == nil || cli.priv == nil || cli.peerID == "" {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ch:
					continue
				}
			}

			state, err := cli.GetAccountState(ctx)
			if err != nil {
				if isNonRetryableCloudError(err) {
					if isUnauthCloudError(err) {
						if err := a.waitAccountFetcherReauth(ctx, err); err != nil {
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

			// Fetch emails alongside account state (same epoch invalidation).
			emailResp, err := cli.ListEmails(ctx)
			if err != nil {
				if isNonRetryableCloudError(err) {
					if isUnauthCloudError(err) {
						if err := a.waitAccountFetcherReauth(ctx, err); err != nil {
							return err
						}
						continue
					}
					le.WithError(err).Warn("permanent error fetching emails")
					return err
				}
				le.WithError(err).Warn("failed to fetch emails, will retry")
				if err := retry.WaitTransient(ctx, ch, err); err != nil {
					return err
				}
				continue
			}

			sessionRows, err := cli.ListSessions(ctx)
			if err != nil {
				if isNonRetryableCloudError(err) {
					if isUnauthCloudError(err) {
						if err := a.waitAccountFetcherReauth(ctx, err); err != nil {
							return err
						}
						continue
					}
					le.WithError(err).Warn("permanent error fetching sessions")
					return err
				}
				le.WithError(err).Warn("failed to fetch sessions, will retry")
				if err := retry.WaitTransient(ctx, ch, err); err != nil {
					return err
				}
				continue
			}
			retry.Reset()

			le.WithFields(logrus.Fields{
				"epoch":         state.GetEpoch(),
				"keypair-count": state.GetKeypairCount(),
			}).Debug("fetched account state")

			keypairsChanged := !protobuf_go_lite.IsEqualVTSlice(prevKeypairs, state.GetKeypairs())
			prevKeypairs = state.GetKeypairs()

			a.applyFetchedAccountState(epoch, state, emailResp.GetEmails(), sessionRows)
			a.syncSharedObjectListAccess(state.GetSubscriptionStatus())
			a.refreshSelfRejoinSweepState()

			// Write cache to ObjectStore if epoch advanced.
			if uint64(state.GetEpoch()) > lastFetched {
				if err := a.writeAccountStateCache(ctx, state); err != nil {
					le.WithError(err).Warn("failed to write account state cache")
				}
			}

			if keypairsChanged && len(state.GetKeypairs()) > 0 {
				le.Debug("keypairs changed, rewrapping session envelope")
				if err := a.RewrapSessionEnvelope(ctx); err != nil {
					le.WithError(err).Warn("failed to rewrap session envelope after keypair change")
				}
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

func (a *ProviderAccount) waitAccountFetcherReauth(ctx context.Context, err error) error {
	var rejoinState *selfRejoinSweepState
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.state.status != provider.ProviderAccountStatus_ProviderAccountStatus_UNAUTHENTICATED {
			a.state.status = accountstatus.Unauthenticated(a.state.info)
			rejoinState = a.buildSelfRejoinSweepStateLocked()
			broadcast()
		}
	})
	a.setSelfRejoinSweepState(rejoinState)
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

// applyFetchedAccountState stores freshly fetched account state.
//
// If no newer invalidation arrived while the fetch was in flight, collapse the
// local trigger epoch back down to the fetched server epoch. This lets a
// bootstrap fetch at local epoch 1 with server epoch 0 settle to 0 so a later
// remote account_changed(epoch=1) still triggers a refetch.
func (a *ProviderAccount) applyFetchedAccountState(
	startEpoch uint64,
	state *api.AccountStateResponse,
	emails []*api.AccountEmailInfo,
	sessionRows []*api.AccountSessionInfo,
) {
	var reconcileState *sessionPresentationReconcileState
	var rejoinState *selfRejoinSweepState
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.state.info = state
		a.state.status = accountstatus.Loaded(state)
		a.state.accountBootstrapFetched = true
		fetchedEpoch := uint64(state.GetEpoch())
		if a.state.epoch == startEpoch {
			a.state.epoch = fetchedEpoch
		}
		if fetchedEpoch > a.state.lastFetchedEpoch {
			a.state.lastFetchedEpoch = fetchedEpoch
		}
		a.state.cachedEmails = emails
		a.state.cachedEmailsValid = true
		a.state.sessions = sessionRows
		a.state.sessionsValid = true
		a.state.infoFetching = false
		reconcileState = a.buildSessionPresentationReconcileStateLocked()
		rejoinState = a.buildSelfRejoinSweepStateLocked()
		broadcast()
	})
	a.setSessionPresentationReconcileState(reconcileState)
	a.setSelfRejoinSweepState(rejoinState)
}

// writeAccountStateCache serializes AccountStateCache and writes it to ObjectStore.
func (a *ProviderAccount) writeAccountStateCache(ctx context.Context, state *api.AccountStateResponse) error {
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
