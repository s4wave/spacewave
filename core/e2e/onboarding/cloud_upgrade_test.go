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
	// Bound the test by the suite context.
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	// Count the sessions that exist before the test adds its own.
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

	// Create an independent local session with a quickstart Space, the data
	// the user accumulated before registering a cloud account.
	originalLocal, _ := createLocalSession(ctx, t, "")
	spaceName := "Free Tier Space"
	createLocalSpace(ctx, t, originalLocal, spaceName)

	// Register a cloud account. A new account has no subscription and holds
	// the free platform role.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()

	// Access the account through the spacewave provider.
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

	// The fresh account has no active subscription. The coordinator assigns
	// the "free" role while the subscription status is "none" or "lapsed".
	freeStatus, err := swAcc.GetSubscriptionStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if freeStatus == "active" {
		t.Fatalf("expected non-active subscription on fresh cloud account, got %q", freeStatus)
	}

	// Upgrade to an active subscription through the coordinator test
	// helper, which swaps the platform role to "subscriber". Bump the epoch
	// so the provider account refreshes its state.
	setTestSubscriptionStatus(t, cloudAccountID, "active")
	swAcc.BumpLocalEpoch()

	// Wait for the account to read the active subscription.
	activeStatus, err := waitForSubscriptionStatus(ctx, swAcc, "active")
	if err != nil {
		t.Fatalf("waiting for active subscription: %v", err)
	}
	if activeStatus != "active" {
		t.Fatalf("expected active subscription after upgrade, got %q", activeStatus)
	}

	// Build the cloud session's Spacewave resource.
	cloudResource, cloudSess, relCloudResource := mountSessionResource(ctx, t, cloudEntry)
	defer relCloudResource()
	swResource := resource_session.NewSpacewaveSessionResource(
		cloudResource,
		logrus.NewEntry(logrus.StandardLogger()),
		env.tb.Bus,
		cloudSess,
		swAcc,
	)

	// Create the linked local target through the RPC the UI calls after an
	// upgrade.
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

	// The account records the link from the cloud session to the target.
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

	// The list holds the original local, cloud, and linked local sessions.
	beforeTransfer := waitForSessionCount(ctx, t, sessCtrl, initialCount+3)
	if len(beforeTransfer) != initialCount+3 {
		t.Fatalf("expected %d sessions before transfer, got %d", initialCount+3, len(beforeTransfer))
	}

	// Merge the original local session's Space into the linked local
	// target, which removes the source afterward.
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

	// The merge deletes the source local session, leaving the cloud and
	// linked local sessions.
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

	// The target's inventory holds the migrated Space.
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

	// The cloud session stays linked to the same local session.
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
