package provider_spacewave

import (
	"context"

	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/provider/spacewave/accountstatus"
	"github.com/s4wave/spacewave/core/provider/spacewave/managedbacache"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// InvalidateManagedBAsList invalidates the cached managed billing account list.
func (a *ProviderAccount) InvalidateManagedBAsList() {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.getManagedBAsCacheLocked().Invalidate()
		broadcast()
	})
}

// GetManagedBAsSnapshot returns cached managed BAs, fetching on cache miss.
func (a *ProviderAccount) GetManagedBAsSnapshot(
	ctx context.Context,
) ([]*s4wave_provider_spacewave.ManagedBillingAccount, error) {
	var cache *managedbacache.Store
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cache = a.getManagedBAsCacheLocked()
	})
	accounts, err := cache.Get(ctx, a.GetSessionClient())
	if err != nil {
		a.handleManagedBAsFetchError(err)
		return nil, err
	}
	return accounts, nil
}

func (a *ProviderAccount) getManagedBAsCacheLocked() *managedbacache.Store {
	if a.managedBAsCache == nil {
		a.managedBAsCache = managedbacache.NewStore(snapshotRefCountOptions)
	}
	return a.managedBAsCache
}

func (a *ProviderAccount) handleManagedBAsFetchError(err error) {
	if !isUnauthCloudError(err) {
		return
	}
	var status provider.ProviderAccountStatus
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		status = accountstatus.Unauthenticated(a.state.info)
	})
	a.SetAccountStatus(status)
}
