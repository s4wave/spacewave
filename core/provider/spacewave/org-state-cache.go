package provider_spacewave

import (
	"context"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/provider/spacewave/orgstatecache"
)

// getOrganizationStateCacheLocked returns the organization state cache.
func (a *ProviderAccount) getOrganizationStateCacheLocked() *orgstatecache.Store {
	if a.orgStateCache == nil {
		a.orgStateCache = orgstatecache.NewStore(snapshotRefCountOptions)
	}
	return a.orgStateCache
}

// invalidateOrganizationSnapshotsLocked invalidates one or all org snapshots.
func (a *ProviderAccount) invalidateOrganizationSnapshotsLocked(orgID string) {
	if a.orgStateCache != nil {
		a.orgStateCache.Invalidate(orgID)
	}
}

// InvalidateOrganizationState invalidates a cached organization snapshot.
func (a *ProviderAccount) InvalidateOrganizationState(orgID string) {
	a.orgBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.invalidateOrganizationSnapshotsLocked(orgID)
		broadcast()
	})
}

// GetOrganizationSnapshot returns a cached org detail snapshot, fetching on miss.
func (a *ProviderAccount) GetOrganizationSnapshot(
	ctx context.Context,
	orgID string,
) (*api.GetOrgResponse, *api.ListOrgInvitesResponse, string, error) {
	if orgID == "" {
		return nil, nil, "", errors.New("organization id is required")
	}

	var cache *orgstatecache.Store
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cache = a.getOrganizationStateCacheLocked()
	})
	return cache.Get(ctx, orgID, a.GetSessionClient(), a.getOrganizationRole)
}

// getOrganizationRole returns the cached or fetched role for an organization.
func (a *ProviderAccount) getOrganizationRole(
	ctx context.Context,
	orgID string,
) (string, error) {
	orgSummary := a.GetCachedOrganization(orgID)
	if orgSummary == nil {
		orgs, err := a.getOrganizationList(ctx)
		if err != nil {
			return "", err
		}
		for _, org := range orgs {
			if org.GetId() == orgID {
				orgSummary = org
				break
			}
		}
	}
	if orgSummary != nil {
		return orgSummary.GetRole(), nil
	}
	return "", nil
}
