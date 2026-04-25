package provider_local

import (
	"context"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

// WatchOrgList watches the organization list by scanning the SO list for
// org-typed SOs. Emits updates whenever the SO list changes.
// Returns when the context is canceled.
func (a *ProviderAccount) WatchOrgList(ctx context.Context, cb func([]*api.OrgResponse)) error {
	soList, err := a.soListCtr.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	for {
		var orgs []*api.OrgResponse
		for _, entry := range soList.GetSharedObjects() {
			if entry.GetMeta().GetBodyType() != s4wave_org.OrgBodyType {
				continue
			}
			orgID := entry.GetRef().GetProviderResourceRef().GetId()
			role := "org:owner"
			if entry.GetSource() == "shared" {
				role = "org:member"
			}
			orgs = append(orgs, &api.OrgResponse{
				Id:          orgID,
				DisplayName: s4wave_org.OrgDisplayNameFromMeta(entry.GetMeta()),
				Role:        role,
			})
		}
		cb(orgs)

		soList, err = a.soListCtr.WaitValueChange(ctx, soList, nil)
		if err != nil {
			return err
		}
	}
}
