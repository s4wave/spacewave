package provider_spacewave

import (
	"context"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/billingcache"
)

// InvalidateBillingSnapshot invalidates a cached billing snapshot.
func (a *ProviderAccount) InvalidateBillingSnapshot(baID string) {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.getBillingCacheLocked().Invalidate(baID)
		broadcast()
	})
}

// GetBillingSnapshot returns cached billing state and usage, fetching on cache miss.
func (a *ProviderAccount) GetBillingSnapshot(
	ctx context.Context,
	baID string,
) (*api.BillingStateResponse, *api.BillingUsageResponse, error) {
	// Capture the billing cache under its broadcast lock.
	var cache *billingcache.Store
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cache = a.getBillingCacheLocked()
	})

	// Keep the cache's nil-fetcher behavior while the account has no signer.
	var fetcher billingcache.Fetcher
	if cli, err := a.GetSessionClient(); err == nil {
		fetcher = cli
	}

	// Let the cache serve or refresh its snapshot.
	return cache.Get(ctx, baID, fetcher)
}

func (a *ProviderAccount) getBillingCacheLocked() *billingcache.Store {
	if a.state.billingCache == nil {
		a.state.billingCache = billingcache.NewStore(snapshotRefCountOptions)
	}
	return a.state.billingCache
}
