//go:build e2e

package onboarding_test

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// waitForSubscriptionStatus polls the spacewave provider account snapshot until
// the cached subscription_status matches want. Used after BumpLocalEpoch to
// observe a freshly-fetched subscription state without writing a watch loop.
func waitForSubscriptionStatus(
	ctx context.Context,
	swAcc *provider_spacewave.ProviderAccount,
	want string,
) (string, error) {
	deadline := time.Now().Add(20 * time.Second)
	var last string
	for {
		state, err := swAcc.GetAccountState(ctx)
		if err != nil {
			return "", err
		}
		last = state.GetSubscriptionStatus().NormalizedString()
		if last == want {
			return last, nil
		}
		if time.Now().After(deadline) {
			return last, errors.Errorf(
				"timed out waiting for subscription status %q, last %q",
				want,
				last,
			)
		}
		swAcc.BumpLocalEpoch()
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// TestCloudFreeToUpgradeFullFlow exercises the cloud-first onboarding path
// where the user registers a cloud account, lives on the free tier briefly,
// then upgrades to an active subscription before linking a local session and
// transferring data from a pre-existing independent local session.
//
// Walks the same path the UI takes when a user signs up cloud-first, decides
// to upgrade later, and pulls quickstart data from another local session into
// the new linked local target.
func TestCloudFreeToUpgradeFullFlow(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	sessCtrl, relSess, err := lookupSessionController(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer relSess()

	initialSessions, err := sessCtrl.ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initialCount := len(initialSessions)

	// 1. Independent local session with a quickstart space (the "free" data
	//    the user accumulated before deciding to register cloud).
	originalLocal, _ := createLocalSession(ctx, t, "")
	spaceName := "Free Tier Space"
	createLocalSpace(ctx, t, originalLocal, spaceName)

	// 2. Register cloud account. The default subscription_status in D1 is
	//    "none" which the cloud RBAC layer treats as the free tier.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()

	prov, provRef, err := provider.ExLookupProvider(ctx, env.tb.Bus, "spacewave", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer provRef.Release()

	swProv := prov.(*provider_spacewave.Provider)
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, cloudAccountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer relAcc()
	swAcc := accIface.(*provider_spacewave.ProviderAccount)

	// 3. Verify free-tier baseline: no active subscription on the fresh
	//    account. The cloud RBAC layer assigns the "free" role on
	//    subscription_status of "none" or "lapsed".
	freeStatus, err := swAcc.GetSubscriptionStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if freeStatus == "active" {
		t.Fatalf("expected non-active subscription on fresh cloud account, got %q", freeStatus)
	}

	// 4. Upgrade to an active subscription via the cloud test endpoint.
	//    This swaps the platform RBAC role to "subscriber" and bumps the
	//    account epoch so the local provider account refreshes its state.
	setTestSubscriptionStatus(t, cloudAccountID, "active")
	swAcc.BumpLocalEpoch()

	// 5. Wait for the bump to propagate then verify active state.
	activeStatus, err := waitForSubscriptionStatus(ctx, swAcc, "active")
	if err != nil {
		t.Fatalf("waiting for active subscription: %v", err)
	}
	if activeStatus != "active" {
		t.Fatalf("expected active subscription after upgrade, got %q", activeStatus)
	}

	// 6. Mount the cloud session resource and create the linked local target
	//    via the same RPC the UI uses post-upgrade.
	cloudResource, cloudSess, relCloudResource := mountSessionResource(ctx, t, cloudEntry)
	defer relCloudResource()

	swResource := resource_session.NewSpacewaveSessionResource(
		cloudResource,
		logrus.NewEntry(logrus.StandardLogger()),
		env.tb.Bus,
		cloudSess,
		swAcc,
	)
	created, err := swResource.CreateLinkedLocalSession(
		ctx,
		&s4wave_provider_spacewave.CreateLinkedLocalSessionRequest{},
	)
	if err != nil {
		t.Fatal(err)
	}
	linkedLocal := created.GetSessionListEntry()
	if linkedLocal == nil {
		t.Fatal("expected linked local session entry from CreateLinkedLocalSession")
	}
	if linkedLocal.GetSessionIndex() == originalLocal.GetSessionIndex() {
		t.Fatal("linked local session must be distinct from independent quickstart session")
	}
	if linkedLocal.GetSessionIndex() == cloudEntry.GetSessionIndex() {
		t.Fatal("linked local session must be distinct from cloud session")
	}

	found, linkedIdx, err := swAcc.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || linkedIdx != linkedLocal.GetSessionIndex() {
		t.Fatalf(
			"expected cloud session %s linked to local idx=%d, got found=%t idx=%d",
			cloudSessionID,
			linkedLocal.GetSessionIndex(),
			found,
			linkedIdx,
		)
	}

	// initialCount + originalLocal + cloud + linkedLocal
	beforeTransfer := waitForSessionCount(ctx, t, sessCtrl, initialCount+3)
	if len(beforeTransfer) != initialCount+3 {
		t.Fatalf("expected %d sessions before transfer, got %d", initialCount+3, len(beforeTransfer))
	}

	// 7. Transfer the original local session's space into the linked local
	//    target via MERGE so the source is removed afterward.
	targetResource, _, relTargetResource := mountSessionResource(ctx, t, linkedLocal)
	defer relTargetResource()

	if _, err := targetResource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: originalLocal.GetSessionIndex(),
		TargetSessionIndex: linkedLocal.GetSessionIndex(),
		Mode:               provider_transfer.TransferMode_TransferMode_MERGE,
	}); err != nil {
		t.Fatal(err)
	}

	xfer := targetResource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer after StartTransfer")
	}
	waitForTransferComplete(t, xfer)

	// MERGE deletes the source local session, leaving cloud + linked local.
	afterTransfer := waitForSessionCount(ctx, t, sessCtrl, initialCount+2)
	if len(afterTransfer) != initialCount+2 {
		t.Fatalf("expected %d sessions after transfer, got %d", initialCount+2, len(afterTransfer))
	}

	srcEntry, err := sessCtrl.GetSessionByIdx(ctx, originalLocal.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	if srcEntry != nil {
		t.Fatal("expected original independent local session to be deleted after merge")
	}

	// 8. Inventory the linked local target and confirm the space migrated.
	inventory, err := targetResource.GetTransferInventory(
		ctx,
		&s4wave_session.GetTransferInventoryRequest{
			SessionIndex: linkedLocal.GetSessionIndex(),
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	hasTransferredSpace := false
	for _, sp := range inventory.GetSpaces() {
		if sp.GetSpaceMeta().GetName() == spaceName {
			hasTransferredSpace = true
			break
		}
	}
	if !hasTransferredSpace {
		t.Fatalf("expected transferred space %q on linked local target", spaceName)
	}

	// 9. Cloud session must remain linked to the same local session.
	found, linkedIdx, err = swAcc.GetLinkedLocalSession(ctx, cloudSessionID)
	if err != nil {
		t.Fatal(err)
	}
	if !found || linkedIdx != linkedLocal.GetSessionIndex() {
		t.Fatalf(
			"expected cloud session to remain linked to local idx=%d, got found=%t idx=%d",
			linkedLocal.GetSessionIndex(),
			found,
			linkedIdx,
		)
	}
}
