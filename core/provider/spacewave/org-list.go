package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// fetchAndUpdateOrgList fetches the org list from the cloud and updates the cache.
func (a *ProviderAccount) fetchAndUpdateOrgList(ctx context.Context) {
	orgs, err := a.fetchOrganizationList(ctx)
	if err != nil {
		a.le.WithError(err).Warn("failed to fetch org list")
		return
	}

	a.orgBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.orgList = orgs
		a.orgListValid = true
		a.invalidateOrganizationSnapshotsLocked("")
		broadcast()
	})
}

// GetOrganizationList returns the cached org list, fetching on cache miss.
func (a *ProviderAccount) GetOrganizationList(
	ctx context.Context,
) ([]*api.OrgResponse, error) {
	return a.getOrganizationList(ctx)
}

// RefreshOrganizationList fetches the latest org list and updates the cache.
func (a *ProviderAccount) RefreshOrganizationList(ctx context.Context) error {
	orgs, err := a.fetchOrganizationList(ctx)
	if err != nil {
		return err
	}

	a.orgBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.orgList = orgs
		a.orgListValid = true
		a.invalidateOrganizationSnapshotsLocked("")
		broadcast()
	})
	return nil
}

// GetCachedSharedObjectOrganizationID returns the cached owner org id for an SO.
func (a *ProviderAccount) GetCachedSharedObjectOrganizationID(
	soID string,
) (string, bool) {
	if soID == "" {
		return "", false
	}

	var orgID string
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !a.orgListValid {
			return
		}
		for _, org := range a.orgList {
			for _, spaceID := range org.GetSpaceIds() {
				if spaceID != soID {
					continue
				}
				orgID = org.GetId()
				return
			}
		}
	})
	return orgID, orgID != ""
}

// fetchOrganizationList fetches the org list from the cloud without storing it.
func (a *ProviderAccount) fetchOrganizationList(
	ctx context.Context,
) ([]*api.OrgResponse, error) {
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, errors.New("session client not available")
	}

	data, err := cli.ListOrganizations(ctx)
	if err != nil {
		return nil, err
	}

	var resp api.ListOrgsResponse
	if err := resp.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "unmarshal organization list")
	}
	return resp.GetOrganizations(), nil
}

// WatchOrgList watches the organization list, emitting updates when membership changes.
// The callback is called with the current org list whenever it changes.
// Returns when the context is canceled.
func (a *ProviderAccount) WatchOrgList(ctx context.Context, cb func([]*api.OrgResponse)) error {
	// Trigger initial fetch if not yet loaded.
	var valid bool
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		valid = a.orgListValid
	})
	if !valid {
		go a.fetchAndUpdateOrgList(ctx)
	}

	for {
		var ch <-chan struct{}
		var list []*api.OrgResponse
		var listValid bool
		a.orgBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			list = a.orgList
			listValid = a.orgListValid
		})
		if listValid {
			cb(list)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// HasCachedOwnerOrganization returns true when the cached org list contains the
// organization and this account is an owner.
func (a *ProviderAccount) HasCachedOwnerOrganization(orgID string) bool {
	if orgID == "" {
		return false
	}

	var isOwner bool
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !a.orgListValid {
			return
		}
		for _, org := range a.orgList {
			if org.GetId() != orgID {
				continue
			}
			isOwner = isOrganizationOwnerRole(org.GetRole())
			return
		}
	})
	return isOwner
}
