//go:build e2e

package onboarding_test

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
)

// TestSubscriptionLapseReactivateTransferFullFlow exercises the lapse and
// reactivate path: an active cloud account lapses to the free role and keeps
// its session without a subscription, the user keeps working on an
// independent local session, then the subscription is reactivated. After
// reactivation a linked local session is created, and the independent local
// session is merged into the linked local target.
func TestSubscriptionLapseReactivateTransferFullFlow(t *testing.T) {
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

	// Register a cloud account and activate its subscription.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()
	setTestSubscriptionStatus(t, cloudAccountID, "active")

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

	// Wait for the account to read the active subscription.
	swAcc.BumpLocalEpoch()
	if _, err := waitForSubscriptionStatus(ctx, swAcc, "active"); err != nil {
		t.Fatalf("waiting for initial active subscription: %v", err)
	}

	// Create the independent local session the user keeps working in once
	// the cloud subscription lapses.
	originalLocal, _ := createLocalSession(ctx, t, "")
	spaceName := "Reactivated Space"
	createLocalSpace(ctx, t, originalLocal, spaceName)

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

	// Watch the onboarding status through the lapse and reactivation.
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	strm := newOnboardingStatusWatchStream(watchCtx)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- swResource.WatchOnboardingStatus(
			&s4wave_provider_spacewave.WatchOnboardingStatusRequest{},
			strm,
		)
	}()
	ready := provider.ProviderAccountStatus_ProviderAccountStatus_READY
	waitForOnboardingStatus(t, strm.msgs, ready, true)

	// Lapse the subscription. The platform role falls back to "free", which
	// keeps Session.create, so the session stays ready without a
	// subscription.
	setTestSubscriptionStatus(t, cloudAccountID, "lapsed")
	swAcc.BumpLocalEpoch()
	waitForOnboardingStatus(t, strm.msgs, ready, false)

	// Reactivate. The platform role returns to "subscriber".
	setTestSubscriptionStatus(t, cloudAccountID, "active")
	swAcc.BumpLocalEpoch()
	waitForOnboardingStatus(t, strm.msgs, ready, true)

	// Stop the watch and confirm the account reads the reactivation.
	watchCancel()
	if err := <-watchErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := waitForSubscriptionStatus(ctx, swAcc, "active"); err != nil {
		t.Fatalf("waiting for reactivated subscription: %v", err)
	}

	// Create the linked local target now that the cloud session is alive
	// again.
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
		t.Fatal("linked local session must be distinct from independent local")
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

	// Merge the independent local session into the linked local target.
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

	// The merge deletes the original local session.
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

	// The target's inventory holds the transferred Space.
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
}
