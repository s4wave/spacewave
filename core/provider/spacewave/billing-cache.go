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
	var cache *billingcache.Store
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cache = a.getBillingCacheLocked()
	})
	return cache.Get(ctx, baID, a.GetSessionClient())
}

func (a *ProviderAccount) getBillingCacheLocked() *billingcache.Store {
	if a.state.billingCache == nil {
		a.state.billingCache = billingcache.NewStore(snapshotRefCountOptions)
	}
	return a.state.billingCache
}
