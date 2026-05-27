package orgprocessor

import (
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_org "github.com/s4wave/spacewave/sdk/org"
)

// Keys returns organization SO IDs owned by the cached org list.
func Keys(soList *sobject.SharedObjectList, orgs []*api.OrgResponse, valid bool) []string {
	if soList == nil || !valid {
		return nil
	}

	ownerOrgs := make(map[string]struct{})
	for _, org := range orgs {
		if isOrganizationOwnerRole(org.GetRole()) {
			ownerOrgs[org.GetId()] = struct{}{}
		}
	}
	if len(ownerOrgs) == 0 {
		return nil
	}

	var orgIDs []string
	for _, entry := range soList.GetSharedObjects() {
		if entry.GetMeta().GetBodyType() != s4wave_org.OrgBodyType {
			continue
		}
		orgID := entry.GetRef().GetProviderResourceRef().GetId()
		if _, ok := ownerOrgs[orgID]; !ok {
			continue
		}
		orgIDs = append(orgIDs, orgID)
	}
	return orgIDs
}

func isOrganizationOwnerRole(roleID string) bool {
	return roleID == "owner" || roleID == "org:owner"
}
