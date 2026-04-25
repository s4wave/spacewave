package resource_session

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

func TestBuildOrganizationRootStateInfoPropagatesHealthAndOwnerPermissions(t *testing.T) {
	t.Parallel()

	health := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_INITIAL_STATE_REJECTED,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_CONTACT_OWNER,
		"root signature validation failed",
	)

	rootState := buildOrganizationRootStateInfo("org-1", health, "org:owner")
	if rootState == nil {
		t.Fatal("expected root state info")
	}
	if rootState.GetSharedObjectId() != "org-1" {
		t.Fatalf("unexpected shared object id: %q", rootState.GetSharedObjectId())
	}
	if rootState.GetHealth().GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_INITIAL_STATE_REJECTED {
		t.Fatalf("unexpected health reason: %+v", rootState.GetHealth())
	}
	if rootState.GetHealth().GetError() != "root signature validation failed" {
		t.Fatalf("unexpected health error: %q", rootState.GetHealth().GetError())
	}
	if !rootState.GetMutationPermission().GetCanRepair() {
		t.Fatal("expected owner repair permission")
	}
	if !rootState.GetMutationPermission().GetCanReinitialize() {
		t.Fatal("expected owner reinitialize permission")
	}
	if rootState.GetMutationPermission().GetDisabledReason() != "" {
		t.Fatalf(
			"unexpected disabled reason: %q",
			rootState.GetMutationPermission().GetDisabledReason(),
		)
	}
}

func TestBuildOrganizationRootStateInfoDisablesMutationsForMembers(t *testing.T) {
	t.Parallel()

	rootState := buildOrganizationRootStateInfo(
		"org-1",
		sobject.NewSharedObjectReadyHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
		"org:member",
	)
	if rootState == nil {
		t.Fatal("expected root state info")
	}
	if rootState.GetMutationPermission().GetCanRepair() {
		t.Fatal("expected member repair to be disabled")
	}
	if rootState.GetMutationPermission().GetCanReinitialize() {
		t.Fatal("expected member reinitialize to be disabled")
	}
	if got := rootState.GetMutationPermission().GetDisabledReason(); got == "" {
		t.Fatal("expected disabled reason")
	}
}
