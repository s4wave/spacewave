package rootstate

import (
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// BuildInfo builds the organization root-state response projection.
func BuildInfo(
	sharedObjectID string,
	health *sobject.SharedObjectHealth,
	roleID string,
) *s4wave_provider_spacewave.OrganizationRootStateInfo {
	if sharedObjectID == "" {
		return nil
	}
	canMutate := roleID == "org:owner" || roleID == "owner"
	return &s4wave_provider_spacewave.OrganizationRootStateInfo{
		SharedObjectId: sharedObjectID,
		Health:         health,
		MutationPermission: &s4wave_provider_spacewave.SharedObjectMutationPermission{
			CanRepair:       canMutate,
			CanReinitialize: canMutate,
			DisabledReason:  mutationDisabledReason(roleID),
		},
	}
}

func mutationDisabledReason(roleID string) string {
	if roleID == "org:owner" || roleID == "owner" {
		return ""
	}
	return "Only organization owners can repair or reinitialize this shared object."
}
