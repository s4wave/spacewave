package rootstate

import (
	"testing"

	"github.com/s4wave/spacewave/core/sobject"
)

func TestBuildInfoPropagatesHealthAndOwnerPermissions(t *testing.T) {
	// Run the root-state permission checks independently.
	t.Parallel()

	// Build rejected-root health evidence for the organization owner.
	health := sobject.NewSharedObjectClosedHealth(
		sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_INITIAL_STATE_REJECTED,
		sobject.SharedObjectHealthRemediationHint_SHARED_OBJECT_HEALTH_REMEDIATION_HINT_CONTACT_OWNER,
		"root signature validation failed",
	)

	// Project the rejected health state into the owner's root-state response.
	rootState := BuildInfo("org-1", health, "org:owner")

	// Assert that the root-state projection exists.
	if rootState == nil {
		t.Fatal("expected root state info")
	}

	// Assert that the projection identifies the requested organization root.
	if rootState.GetSharedObjectId() != "org-1" {
		t.Fatalf("unexpected shared object id: %q", rootState.GetSharedObjectId())
	}

	// Assert that health preserves the rejected initial-state reason.
	if rootState.GetHealth().GetCommonReason() != sobject.SharedObjectHealthCommonReason_SHARED_OBJECT_HEALTH_COMMON_REASON_INITIAL_STATE_REJECTED {
		t.Fatalf("unexpected health reason: %+v", rootState.GetHealth())
	}

	// Assert that health preserves its diagnostic error text.
	if rootState.GetHealth().GetError() != "root signature validation failed" {
		t.Fatalf("unexpected health error: %q", rootState.GetHealth().GetError())
	}

	// Assert that an owner can repair the root SharedObject.
	if !rootState.GetMutationPermission().GetCanRepair() {
		t.Fatal("expected owner repair permission")
	}

	// Assert that an owner can reinitialize the root SharedObject.
	if !rootState.GetMutationPermission().GetCanReinitialize() {
		t.Fatal("expected owner reinitialize permission")
	}

	// Assert that the owner has no disabled mutation reason.
	if rootState.GetMutationPermission().GetDisabledReason() != "" {
		t.Fatalf(
			"unexpected disabled reason: %q",
			rootState.GetMutationPermission().GetDisabledReason(),
		)
	}
}

func TestBuildInfoDisablesMutationsForMembers(t *testing.T) {
	// Run the member permission check independently.
	t.Parallel()

	// Build ready root-state data for an organization member.
	rootState := BuildInfo(
		"org-1",
		sobject.NewSharedObjectReadyHealth(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
		),
		"org:member",
	)

	// Assert that the member root-state projection exists.
	if rootState == nil {
		t.Fatal("expected root state info")
	}

	// Assert that a member cannot repair the root SharedObject.
	if rootState.GetMutationPermission().GetCanRepair() {
		t.Fatal("expected member repair to be disabled")
	}

	// Assert that a member cannot reinitialize the root SharedObject.
	if rootState.GetMutationPermission().GetCanReinitialize() {
		t.Fatal("expected member reinitialize to be disabled")
	}

	// Assert that the member receives a disabled-mutation reason.
	if got := rootState.GetMutationPermission().GetDisabledReason(); got == "" {
		t.Fatal("expected disabled reason")
	}
}
