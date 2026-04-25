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

// TestSubscriptionLapseReactivateTransferFullFlow exercises the lapse +
// reactivate path: an active cloud account lapses to dormant, the user keeps
// working on an independent local session, then the subscription is
// reactivated. After reactivation the dormant tracker wakes, a linked local
// session is created, and the independent local session is merged into the
// linked local target.
func TestSubscriptionLapseReactivateTransferFullFlow(t *testing.T) {
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

	// 1. Register cloud account and immediately activate.
	cloudEntry := createCloudSession(ctx, t)
	cloudRef := cloudEntry.GetSessionRef().GetProviderResourceRef()
	cloudAccountID := cloudRef.GetProviderAccountId()
	cloudSessionID := cloudRef.GetId()
	setTestSubscriptionStatus(t, cloudAccountID, "active")

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
	swAcc.BumpLocalEpoch()

	if _, err := waitForSubscriptionStatus(ctx, swAcc, "active"); err != nil {
		t.Fatalf("waiting for initial active subscription: %v", err)
	}

	// 2. Independent local session that the user keeps working in once the
	//    cloud subscription lapses to dormant.
	originalLocal, _ := createLocalSession(ctx, t, "")
	spaceName := "Reactivated Space"
	createLocalSpace(ctx, t, originalLocal, spaceName)

	// 3. Start watching onboarding status to observe DORMANT then READY
	//    transitions through the lapse + reactivate dance.
	cloudResource, cloudSess, relCloudResource := mountSessionResource(ctx, t, cloudEntry)
	defer relCloudResource()

	swResource := resource_session.NewSpacewaveSessionResource(
		cloudResource,
		logrus.NewEntry(logrus.StandardLogger()),
		env.tb.Bus,
		cloudSess,
		swAcc,
	)

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

	// Drain initial READY emission so the dormant wait observes the
	// transition rather than the pre-lapse state.
	drainOnboardingStatusUntil(
		strm.msgs,
		provider.ProviderAccountStatus_ProviderAccountStatus_READY,
	)

	// 4. Lapse the subscription. The cloud RBAC layer flips the platform role
	//    to "free" which removes Session.create access. The session ticket
	//    flow returns rbac_denied and the tracker enters DORMANT.
	setTestSubscriptionStatus(t, cloudAccountID, "lapsed")
	swAcc.BumpLocalEpoch()

	dormantResp := waitForDormantOnboardingStatus(t, strm.msgs)
	if dormantResp.GetHasSubscription() {
		t.Fatal("expected dormant onboarding status to report no subscription")
	}

	// 5. Reactivate. setTestSubscriptionStatus("active") swaps the role back
	//    to "subscriber", which the dormant tracker observes via the account
	//    broadcast and exits DORMANT.
	setTestSubscriptionStatus(t, cloudAccountID, "active")
	swAcc.BumpLocalEpoch()

	readyResp := waitForReadyOnboardingStatus(t, strm.msgs)
	if !readyResp.GetHasSubscription() {
		t.Fatal("expected ready onboarding status to report active subscription")
	}

	watchCancel()
	if err := <-watchErr; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}

	if _, err := waitForSubscriptionStatus(ctx, swAcc, "active"); err != nil {
		t.Fatalf("waiting for reactivated subscription: %v", err)
	}

	// 6. Create the linked local target now that the cloud session is alive
	//    again, and pull the original independent local session in via merge.
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

// drainOnboardingStatusUntil consumes onboarding status messages from the
// channel non-blocking until one matching want is observed or the channel has
// no further pending messages. Used to skip pre-existing emissions before
// asserting on a state transition.
func drainOnboardingStatusUntil(
	msgs <-chan *s4wave_provider_spacewave.WatchOnboardingStatusResponse,
	want provider.ProviderAccountStatus,
) {
	for {
		select {
		case resp := <-msgs:
			if resp.GetAccountStatus() == want {
				return
			}
		default:
			return
		}
	}
}
