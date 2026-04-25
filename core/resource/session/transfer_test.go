package resource_session_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

// testEnv holds the shared test environment for session resource tests.
type testEnv struct {
	tb          *testbed.Testbed
	prov        *provider_local.Provider
	sessCtrl    session.SessionController
	sessCtrlRel func()
}

// setupTestEnv creates a testbed with local provider and session controller.
func setupTestEnv(ctx context.Context, t *testing.T) *testEnv {
	t.Helper()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}

	tb.StaticResolver.AddFactory(session_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus))

	peerID := tb.Volume.GetPeerID()
	providerID := "local"

	// Start session controller.
	_, sessCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&session_controller.Config{
		VolumeId: tb.EngineVolumeID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessCtrlRef.Release)

	// Start local provider.
	_, provCtrlRef, err := tb.Bus.AddDirective(resolver.NewLoadControllerWithConfig(&provider_local.Config{
		ProviderId: providerID,
		PeerId:     peerID.String(),
		StorageId:  tb.StorageID,
	}), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provCtrlRef.Release)

	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, providerID, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)

	sessCtrl, sessCtrlLookupRef, err := session.ExLookupSessionController(ctx, tb.Bus, "", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessCtrlLookupRef.Release)

	return &testEnv{
		tb:       tb,
		prov:     prov.(*provider_local.Provider),
		sessCtrl: sessCtrl,
	}
}

// createSession creates a local account+session and registers it.
// Returns the session ref and session index.
func (e *testEnv) createSession(ctx context.Context, t *testing.T) (*session.SessionRef, uint32) {
	t.Helper()

	sessRef, err := e.prov.CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}

	entry, err := e.sessCtrl.RegisterSession(ctx, sessRef, &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   sessRef.GetProviderResourceRef().GetProviderAccountId(),
	})
	if err != nil {
		t.Fatal(err)
	}

	return sessRef, entry.GetSessionIndex()
}

// accessAccount gets the provider account for a session, registering a cleanup release.
func (e *testEnv) accessAccount(ctx context.Context, t *testing.T, sessRef *session.SessionRef) *provider_local.ProviderAccount {
	t.Helper()

	accountID := sessRef.GetProviderResourceRef().GetProviderAccountId()
	accIface, accRel, err := e.prov.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(accRel)
	return accIface.(*provider_local.ProviderAccount)
}

// createSpaceOnAccount creates a space with the given name on the provider account.
func (e *testEnv) createSpaceOnAccount(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, spaceName string) {
	t.Helper()

	meta, err := space.NewSharedObjectMeta(spaceName)
	if err != nil {
		t.Fatal(err)
	}
	soID := strings.ToLower(spaceName) + "-id"
	if _, err := acc.CreateSharedObject(ctx, soID, meta, "", ""); err != nil {
		t.Fatal(err)
	}
}

// buildSessionResource creates a SessionResource for the given session ref.
func (e *testEnv) buildSessionResource(ctx context.Context, t *testing.T, sessRef *session.SessionRef) *resource_session.SessionResource {
	t.Helper()

	sess, sessRelRef, err := session.ExMountSession(ctx, e.tb.Bus, sessRef, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(sessRelRef.Release)

	le := logrus.NewEntry(logrus.StandardLogger())
	return resource_session.NewSessionResource(le, e.tb.Bus, sess)
}

// TestGetTransferInventory verifies that GetTransferInventory returns the space list
// for a given session index.
func TestGetTransferInventory(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	// Create a session with 2 spaces.
	sessRef, sessIdx := env.createSession(ctx, t)
	acc := env.accessAccount(ctx, t, sessRef)
	env.createSpaceOnAccount(ctx, t, acc, "Notes")
	env.createSpaceOnAccount(ctx, t, acc, "Projects")

	// Create a second session to serve as "our" session for making the RPC call.
	ourRef, _ := env.createSession(ctx, t)
	resource := env.buildSessionResource(ctx, t, ourRef)

	// Call GetTransferInventory targeting the first session.
	resp, err := resource.GetTransferInventory(ctx, &s4wave_session.GetTransferInventoryRequest{
		SessionIndex: sessIdx,
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(resp.GetSpaces()) != 2 {
		t.Fatalf("expected 2 spaces, got %d", len(resp.GetSpaces()))
	}

	// Verify space names are present.
	names := make(map[string]bool)
	for _, sp := range resp.GetSpaces() {
		names[sp.GetSpaceMeta().GetName()] = true
	}
	if !names["Notes"] || !names["Projects"] {
		t.Fatalf("expected spaces Notes and Projects, got %v", names)
	}
}

// TestStartTransfer verifies that StartTransfer initiates a merge between sessions.
func TestStartTransfer(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "MySpace")

	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef) // keep alive
	resource := env.buildSessionResource(ctx, t, tgtRef)

	_, err := resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               provider_transfer.TransferMode_TransferMode_MERGE,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for completion by polling state.
	xfer := resource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer")
	}
	for {
		ch := xfer.WaitState()
		state := xfer.GetState()
		if state.GetPhase() == provider_transfer.TransferPhase_TransferPhase_COMPLETE {
			break
		}
		if state.GetPhase() == provider_transfer.TransferPhase_TransferPhase_FAILED {
			t.Fatalf("transfer failed: %s", state.GetErrorMessage())
		}
		<-ch
	}

	// Verify target has the space.
	invResp, err := resource.GetTransferInventory(ctx, &s4wave_session.GetTransferInventoryRequest{
		SessionIndex: tgtIdx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(invResp.GetSpaces()) != 1 {
		t.Fatalf("expected 1 space on target, got %d", len(invResp.GetSpaces()))
	}
}

// TestWatchTransferProgress verifies that WatchTransferProgress receives phase transitions.
func TestWatchTransferProgress(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "WatchSpace")

	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef) // keep alive
	resource := env.buildSessionResource(ctx, t, tgtRef)

	_, err := resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               provider_transfer.TransferMode_TransferMode_MERGE,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Collect states via WatchTransferProgress using a mock stream.
	strm := newMockWatchStream(ctx)
	go func() {
		_ = resource.WatchTransferProgress(&s4wave_session.WatchTransferProgressRequest{}, strm)
		close(strm.done)
	}()

	// Collect phases until COMPLETE.
	phases := make(map[provider_transfer.TransferPhase]bool)
	for {
		select {
		case resp, ok := <-strm.msgs:
			if !ok {
				goto done
			}
			phases[resp.GetState().GetPhase()] = true
			if resp.GetState().GetPhase() == provider_transfer.TransferPhase_TransferPhase_COMPLETE {
				goto done
			}
		case <-strm.done:
			goto done
		}
	}
done:
	if !phases[provider_transfer.TransferPhase_TransferPhase_COMPLETE] {
		t.Fatalf("never received COMPLETE phase, got phases: %v", phases)
	}
}

// TestCancelTransfer verifies that CancelTransfer stops a mid-copy transfer.
func TestCancelTransfer(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "CancelSpace")

	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef) // keep alive
	resource := env.buildSessionResource(ctx, t, tgtRef)

	_, err := resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               provider_transfer.TransferMode_TransferMode_MERGE,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cancel immediately.
	_, err = resource.CancelTransfer(ctx, &s4wave_session.CancelTransferRequest{})
	if err != nil && err.Error() != "no transfer in progress" {
		t.Fatal(err)
	}

	// Verify the transfer is done. A tiny in-memory transfer may complete before
	// CancelTransfer observes it, but it must leave a final state behind.
	xfer := resource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer reference")
	}
	state := xfer.GetState()
	phase := state.GetPhase()
	// After cancellation, transfer may be COMPLETE (if it finished fast) or FAILED (if canceled mid-operation).
	if phase != provider_transfer.TransferPhase_TransferPhase_COMPLETE &&
		phase != provider_transfer.TransferPhase_TransferPhase_FAILED {
		t.Fatalf("expected COMPLETE or FAILED after cancel, got %s", phase)
	}
}

// TestLocalToCloudMigrate verifies MIGRATE mode transfers spaces and cleans up source.
// Uses local-to-local as a stand-in; the cloud target is tested separately.
func TestLocalToCloudMigrate(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "MigrateSpace")

	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef) // keep alive
	resource := env.buildSessionResource(ctx, t, tgtRef)

	_, err := resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               provider_transfer.TransferMode_TransferMode_MIGRATE,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for completion.
	xfer := resource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer")
	}
	for {
		ch := xfer.WaitState()
		state := xfer.GetState()
		if state.GetPhase() == provider_transfer.TransferPhase_TransferPhase_COMPLETE {
			break
		}
		if state.GetPhase() == provider_transfer.TransferPhase_TransferPhase_FAILED {
			t.Fatalf("transfer failed: %s", state.GetErrorMessage())
		}
		<-ch
	}

	// Verify target has the space.
	invResp, err := resource.GetTransferInventory(ctx, &s4wave_session.GetTransferInventoryRequest{
		SessionIndex: tgtIdx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(invResp.GetSpaces()) != 1 {
		t.Fatalf("expected 1 space on target, got %d", len(invResp.GetSpaces()))
	}

	// Verify source spaces were cleaned up (inventory should be empty or session gone).
	srcInv, err := resource.GetTransferInventory(ctx, &s4wave_session.GetTransferInventoryRequest{
		SessionIndex: srcIdx,
	})
	if err == nil && len(srcInv.GetSpaces()) > 0 {
		t.Fatalf("expected source spaces to be cleaned up, got %d", len(srcInv.GetSpaces()))
	}
}

// TestMirror verifies MIRROR mode copies spaces without deleting the source.
func TestMirror(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "MirrorSpace")

	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef)
	resource := env.buildSessionResource(ctx, t, tgtRef)

	_, err := resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               provider_transfer.TransferMode_TransferMode_MIRROR,
	})
	if err != nil {
		t.Fatal(err)
	}

	xfer := resource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer")
	}
	for {
		ch := xfer.WaitState()
		state := xfer.GetState()
		if state.GetPhase() == provider_transfer.TransferPhase_TransferPhase_COMPLETE {
			break
		}
		if state.GetPhase() == provider_transfer.TransferPhase_TransferPhase_FAILED {
			t.Fatalf("transfer failed: %s", state.GetErrorMessage())
		}
		<-ch
	}

	// Target should have the space.
	tgtInv, err := resource.GetTransferInventory(ctx, &s4wave_session.GetTransferInventoryRequest{
		SessionIndex: tgtIdx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(tgtInv.GetSpaces()) != 1 {
		t.Fatalf("expected 1 space on target, got %d", len(tgtInv.GetSpaces()))
	}

	// Source should still have the space (no cleanup in MIRROR mode).
	srcInv, err := resource.GetTransferInventory(ctx, &s4wave_session.GetTransferInventoryRequest{
		SessionIndex: srcIdx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(srcInv.GetSpaces()) != 1 {
		t.Fatalf("expected 1 space on source, got %d", len(srcInv.GetSpaces()))
	}
}

// TestReadLinkedCloudAccountID verifies that readLinkedCloudAccountID detects
// a linked-cloud key on a local session.
func TestReadLinkedCloudAccountID(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	sessRef, sessIdx := env.createSession(ctx, t)
	acc := env.accessAccount(ctx, t, sessRef)

	// Write a linked-cloud key to the session's ObjectStore.
	provRef := sessRef.GetProviderResourceRef()
	provID := provRef.GetProviderId()
	accountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()
	objectStoreID := provider_local.SessionObjectStoreID(provID, accountID)
	volID := acc.GetVolume().GetID()

	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, env.tb.Bus, false, objectStoreID, volID, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer diRef.Release()

	otx, err := objStoreHandle.GetObjectStore().NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	cloudAccountID := "cloud-account-12345"
	if err := otx.Set(ctx, []byte(sessionID+"/linked-cloud"), []byte(cloudAccountID)); err != nil {
		otx.Discard()
		t.Fatal(err)
	}
	if err := otx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Look up the session entry.
	entry, err := env.sessCtrl.GetSessionByIdx(ctx, sessIdx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify readLinkedCloudAccountID detects the cloud link.
	src := provider_transfer.NewLocalTransferSource(acc, provID, accountID, env.tb.Bus)
	got, err := resource_session.ReadLinkedCloudAccountID(ctx, env.tb.Bus, entry, src)
	if err != nil {
		t.Fatal(err)
	}
	if got != cloudAccountID {
		t.Fatalf("expected cloud account ID %q, got %q", cloudAccountID, got)
	}

	// Verify a session without the key returns empty.
	otherRef, otherIdx := env.createSession(ctx, t)
	otherAcc := env.accessAccount(ctx, t, otherRef)
	otherEntry, err := env.sessCtrl.GetSessionByIdx(ctx, otherIdx)
	if err != nil {
		t.Fatal(err)
	}
	otherProvRef := otherRef.GetProviderResourceRef()
	otherSrc := provider_transfer.NewLocalTransferSource(otherAcc, otherProvRef.GetProviderId(), otherProvRef.GetProviderAccountId(), env.tb.Bus)
	got2, err := resource_session.ReadLinkedCloudAccountID(ctx, env.tb.Bus, otherEntry, otherSrc)
	if err != nil {
		t.Fatal(err)
	}
	if got2 != "" {
		t.Fatalf("expected empty cloud account ID for unlinked session, got %q", got2)
	}
}

// TestMergeLinkedSessionReportsCleanupFailure verifies that merging a local
// session with a linked-cloud key returns the cleanup failure through transfer
// state when the linked cloud session is unavailable.
func TestMergeLinkedSessionReportsCleanupFailure(t *testing.T) {
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "LinkedSpace")

	// Write a linked-cloud key to the source session's ObjectStore.
	provRef := srcRef.GetProviderResourceRef()
	provID := provRef.GetProviderId()
	accountID := provRef.GetProviderAccountId()
	sessionID := provRef.GetId()
	objectStoreID := provider_local.SessionObjectStoreID(provID, accountID)
	volID := srcAcc.GetVolume().GetID()

	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, env.tb.Bus, false, objectStoreID, volID, nil)
	if err != nil {
		t.Fatal(err)
	}
	otx, err := objStoreHandle.GetObjectStore().NewTransaction(ctx, true)
	if err != nil {
		diRef.Release()
		t.Fatal(err)
	}
	if err := otx.Set(ctx, []byte(sessionID+"/linked-cloud"), []byte("cloud-account-xyz")); err != nil {
		otx.Discard()
		diRef.Release()
		t.Fatal(err)
	}
	if err := otx.Commit(ctx); err != nil {
		diRef.Release()
		t.Fatal(err)
	}
	diRef.Release()

	// Verify the linked-cloud key is present before merge.
	entry, err := env.sessCtrl.GetSessionByIdx(ctx, srcIdx)
	if err != nil {
		t.Fatal(err)
	}
	srcSource := provider_transfer.NewLocalTransferSource(srcAcc, provID, accountID, env.tb.Bus)
	cloudAccID, err := resource_session.ReadLinkedCloudAccountID(ctx, env.tb.Bus, entry, srcSource)
	if err != nil {
		t.Fatal(err)
	}
	if cloudAccID != "cloud-account-xyz" {
		t.Fatalf("expected linked cloud account before merge, got %q", cloudAccID)
	}

	// Create target and run merge.
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef)
	resource := env.buildSessionResource(ctx, t, tgtRef)

	_, err = resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               provider_transfer.TransferMode_TransferMode_MERGE,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait for transfer goroutine to complete (includes post-transfer cleanup).
	if err := resource.WaitTransferDone(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify cleanup failure is reported through transfer state.
	xfer := resource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer")
	}
	state := xfer.GetState()
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_FAILED {
		t.Fatalf("expected FAILED phase, got %s", state.GetPhase())
	}
	if !strings.Contains(state.GetErrorMessage(), "cleanup linked-cloud ref") {
		t.Fatalf("expected cleanup failure, got %q", state.GetErrorMessage())
	}

	// Verify source session was not deleted after cleanup failed.
	srcEntry, err := env.sessCtrl.GetSessionByIdx(ctx, srcIdx)
	if err != nil {
		t.Fatal(err)
	}
	if srcEntry == nil {
		t.Fatal("source session should remain when cleanup fails")
	}

	// Verify target has the space.
	inv, err := resource.GetTransferInventory(ctx, &s4wave_session.GetTransferInventoryRequest{
		SessionIndex: tgtIdx,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.GetSpaces()) != 1 {
		t.Fatalf("expected 1 space on target, got %d", len(inv.GetSpaces()))
	}
}
