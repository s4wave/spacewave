package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

type organizationSnapshot struct {
	info    *api.GetOrgResponse
	invites *api.ListOrgInvitesResponse
	roleID  string
}

// getOrganizationSnapshotRcLocked returns the org snapshot cache for orgID.
func (a *ProviderAccount) getOrganizationSnapshotRcLocked(
	orgID string,
) *refcount.RefCount[*organizationSnapshot] {
	if orgID == "" {
		return nil
	}
	return getOrCreateSnapshotRefCount(
		&a.orgSnapshotRcs,
		orgID,
		func(ctx context.Context, key string, released func()) (*organizationSnapshot, func(), error) {
			return a.resolveOrganizationSnapshot(ctx, key, released)
		},
	)
}

// invalidateOrganizationSnapshotsLocked invalidates one or all org snapshots.
func (a *ProviderAccount) invalidateOrganizationSnapshotsLocked(orgID string) {
	if orgID == "" {
		for _, rc := range a.orgSnapshotRcs {
			rc.Invalidate()
		}
		return
	}

	rc := a.getOrganizationSnapshotRcLocked(orgID)
	if rc == nil {
		return
	}
	rc.Invalidate()
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

	var rc *refcount.RefCount[*organizationSnapshot]
	a.orgBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rc = a.getOrganizationSnapshotRcLocked(orgID)
	})
	snapshot, rel, err := rc.Resolve(ctx)
	if err != nil {
		return nil, nil, "", err
	}
	defer rel()
	if snapshot == nil || snapshot.info == nil {
		return nil, nil, "", errors.New("organization snapshot not available")
	}
	var invites *api.ListOrgInvitesResponse
	if snapshot.invites != nil {
		invites = snapshot.invites.CloneVT()
	}
	return snapshot.info.CloneVT(), invites, snapshot.roleID, nil
}

// resolveOrganizationSnapshot fetches an org detail snapshot from the cloud.
func (a *ProviderAccount) resolveOrganizationSnapshot(
	ctx context.Context,
	orgID string,
	_ func(),
) (*organizationSnapshot, func(), error) {
	cli := a.GetSessionClient()
	if cli == nil {
		return nil, nil, errors.New("session client not available")
	}

	data, err := cli.GetOrganization(ctx, orgID)
	if err != nil {
		return nil, nil, err
	}
	info := &api.GetOrgResponse{}
	if err := info.UnmarshalVT(data); err != nil {
		return nil, nil, errors.Wrap(err, "unmarshal org info")
	}

	var roleID string
	orgSummary := a.GetCachedOrganization(orgID)
	if orgSummary == nil {
		orgs, err := a.getOrganizationList(ctx)
		if err != nil {
			return nil, nil, err
		}
		for _, org := range orgs {
			if org.GetId() == orgID {
				orgSummary = org
				break
			}
		}
	}
	if orgSummary != nil {
		roleID = orgSummary.GetRole()
	}

	var invites *api.ListOrgInvitesResponse
	if roleID == "owner" || roleID == "org:owner" {
		inviteData, err := cli.ListOrgInvites(ctx, orgID)
		if err != nil {
			return nil, nil, err
		}
		invites = &api.ListOrgInvitesResponse{}
		if err := invites.UnmarshalVT(inviteData); err != nil {
			return nil, nil, errors.Wrap(err, "unmarshal invite list")
		}
	}

	if invites == nil {
		invites = &api.ListOrgInvitesResponse{}
	}
	return &organizationSnapshot{
		info:    info,
		invites: invites,
		roleID:  roleID,
	}, nil, nil
}
