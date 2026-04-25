package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

type managedBAsSnapshot struct {
	accounts []*s4wave_provider_spacewave.ManagedBillingAccount
}

// getManagedBAsRcLocked returns the managed BA snapshot cache.
func (a *ProviderAccount) getManagedBAsRcLocked() *refcount.RefCount[*managedBAsSnapshot] {
	return getOrCreateSingletonSnapshotRefCount(
		&a.managedBAsRc,
		func(ctx context.Context, released func()) (*managedBAsSnapshot, func(), error) {
			return a.resolveManagedBAs(ctx, released)
		},
	)
}

// getManagedBAsRc returns the managed BA snapshot cache.
func (a *ProviderAccount) getManagedBAsRc() *refcount.RefCount[*managedBAsSnapshot] {
	var rc *refcount.RefCount[*managedBAsSnapshot]
	a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rc = a.getManagedBAsRcLocked()
	})
	return rc
}

// InvalidateManagedBAsList invalidates the cached managed billing account list.
func (a *ProviderAccount) InvalidateManagedBAsList() {
	a.accountBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		rc := a.getManagedBAsRcLocked()
		if rc != nil {
			rc.Invalidate()
		}
		broadcast()
	})
}

// GetManagedBAsSnapshot returns cached managed BAs, fetching on cache miss.
func (a *ProviderAccount) GetManagedBAsSnapshot(
	ctx context.Context,
) ([]*s4wave_provider_spacewave.ManagedBillingAccount, error) {
	snapshot, rel, err := a.getManagedBAsRc().Resolve(ctx)
	if err != nil {
		return nil, err
	}
	defer rel()
	if snapshot == nil {
		return nil, nil
	}
	accounts := make([]*s4wave_provider_spacewave.ManagedBillingAccount, 0, len(snapshot.accounts))
	for _, account := range snapshot.accounts {
		if account == nil {
			accounts = append(accounts, nil)
			continue
		}
		accounts = append(accounts, account.CloneVT())
	}
	return accounts, nil
}

// resolveManagedBAs fetches the managed BA list from the cloud.
func (a *ProviderAccount) resolveManagedBAs(
	ctx context.Context,
	_ func(),
) (*managedBAsSnapshot, func(), error) {
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, nil, errors.New("session client not available")
	}
	data, err := cli.ListManagedBillingAccounts(ctx)
	if err != nil {
		if isUnauthCloudError(err) {
			var status provider.ProviderAccountStatus
			a.accountBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				status = unauthenticatedAccountStatus(a.state.info)
			})
			a.SetAccountStatus(status)
		}
		return nil, nil, err
	}
	resp := &s4wave_provider_spacewave.ListManagedBillingAccountsResponse{}
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal managed BA list")
	}
	return &managedBAsSnapshot{accounts: resp.GetAccounts()}, nil, nil
}
