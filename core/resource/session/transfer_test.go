package resource_session_test

import (
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestGetTransferInventory verifies that GetTransferInventory returns the space list
// for a given session index.
func TestGetTransferInventory(t *testing.T) {
	// Start a test environment.
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

	// Return both Spaces from the source session.
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
	// Start a test environment.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	// Create a source session with one Space.
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "MySpace")

	// Create the target session and its resource.
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef) // keep alive
	resource := env.buildSessionResource(ctx, t, tgtRef)

	// Start a local-to-local transfer.
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
		state, ch := xfer.WatchState()
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
	// Start a test environment.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	// Create a source session with one Space.
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "WatchSpace")

	// Create the target session and its resource.
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef) // keep alive
	resource := env.buildSessionResource(ctx, t, tgtRef)

	// Start a local-to-local transfer.
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
	// Start a test environment.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	// Create a source session with one Space.
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "CancelSpace")

	// Create the target session and its resource.
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef) // keep alive
	resource := env.buildSessionResource(ctx, t, tgtRef)

	// Start a local-to-local transfer.
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
	// Start a test environment.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	// Create a source session with one Space.
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "MigrateSpace")

	// Create the target session and its resource.
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef) // keep alive
	resource := env.buildSessionResource(ctx, t, tgtRef)

	// Start a migrate transfer.
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
		state, ch := xfer.WatchState()
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
	// Start a test environment.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)

	// Create a source session with one Space.
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "MirrorSpace")

	// Create the target session and its resource.
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef)
	resource := env.buildSessionResource(ctx, t, tgtRef)

	// Start a mirror transfer.
	_, err := resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               provider_transfer.TransferMode_TransferMode_MIRROR,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Wait until the mirror completes.
	xfer := resource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer")
	}
	for {
		state, ch := xfer.WatchState()
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

// TestMergeDeletesEverySourceSession runs a Merge from a source account that
// two sessions name, started from one of them, and checks that neither stays
// listed: a session left over a deleted volume would reopen it empty under a
// fresh key.
func TestMergeDeletesEverySourceSession(t *testing.T) {
	// Create a source account with one Space and a second session on it.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "TwoSessionSpace")
	secondRef := srcRef.CloneVT()
	secondRef.GetProviderResourceRef().Id = ulid.NewULID()
	second, err := env.sessCtrl.RegisterSession(ctx, secondRef, &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   secondRef.GetProviderResourceRef().GetProviderAccountId(),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Merge into a target, started from the source session itself, whose
	// delete stops the session the transfer runs under.
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef)
	resource := env.buildSessionResource(ctx, t, srcRef)
	state := runTransfer(ctx, t, resource, srcIdx, tgtIdx, provider_transfer.TransferMode_TransferMode_MERGE)
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_COMPLETE {
		t.Fatalf("expected COMPLETE, got %s: %s", state.GetPhase(), state.GetErrorMessage())
	}

	// Neither session of the source account stays listed.
	for _, idx := range []uint32{srcIdx, second.GetSessionIndex()} {
		entry, err := env.sessCtrl.GetSessionByIdx(ctx, idx)
		if err != nil {
			t.Fatal(err)
		}
		if entry != nil {
			t.Fatalf("source session %d still listed after the Merge", idx)
		}
	}
}

// TestMergeLinkedSessionCompletesWhenUnlinkFails merges a local session
// linked to a cloud account that no loaded provider serves, and checks that
// the run completes with the source gone: the unlink follows the volume
// delete, so its failure is only logged.
func TestMergeLinkedSessionCompletesWhenUnlinkFails(t *testing.T) {
	// Create a source session with one Space, linked to a cloud account.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "LinkedSpace")
	provRef := srcRef.GetProviderResourceRef()
	objectStoreID := provider_local.SessionObjectStoreID(provRef.GetProviderId(), provRef.GetProviderAccountId())
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, env.tb.Bus, false, objectStoreID, srcAcc.GetVolume().GetID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	otx, err := objStoreHandle.GetObjectStore().NewTransaction(ctx, true)
	if err != nil {
		diRef.Release()
		t.Fatal(err)
	}
	if err := otx.Set(ctx, provider_local.LinkedCloudKey(provRef.GetId()), []byte("cloud-account-xyz")); err != nil {
		otx.Discard()
		diRef.Release()
		t.Fatal(err)
	}
	err = otx.Commit(ctx)
	diRef.Release()
	if err != nil {
		t.Fatal(err)
	}

	// List a cloud session for that account, which no provider can load.
	cloudRef := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		Id:                "cloud-session",
		ProviderId:        "spacewave",
		ProviderAccountId: "cloud-account-xyz",
	}}
	if _, err := env.sessCtrl.RegisterSession(ctx, cloudRef, &session.SessionMetadata{ProviderId: "spacewave"}); err != nil {
		t.Fatal(err)
	}

	// Merge into a target.
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef)
	resource := env.buildSessionResource(ctx, t, tgtRef)
	state := runTransfer(ctx, t, resource, srcIdx, tgtIdx, provider_transfer.TransferMode_TransferMode_MERGE)
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_COMPLETE {
		t.Fatalf("expected COMPLETE, got %s: %s", state.GetPhase(), state.GetErrorMessage())
	}

	// The source session is gone and the target holds the Space.
	srcEntry, err := env.sessCtrl.GetSessionByIdx(ctx, srcIdx)
	if err != nil {
		t.Fatal(err)
	}
	if srcEntry != nil {
		t.Fatal("source session still listed after the Merge")
	}
	inv, err := resource.GetTransferInventory(ctx, &s4wave_session.GetTransferInventoryRequest{SessionIndex: tgtIdx})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.GetSpaces()) != 1 {
		t.Fatalf("expected 1 space on target, got %d", len(inv.GetSpaces()))
	}
}

// runTransfer starts a transfer on resource and returns its state once its
// routine, including the source account delete, has exited.
func runTransfer(
	ctx context.Context,
	t *testing.T,
	resource *resource_session.SessionResource,
	srcIdx, tgtIdx uint32,
	mode provider_transfer.TransferMode,
) *provider_transfer.TransferState {
	t.Helper()
	_, err := resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               mode,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := resource.WaitTransferDone(ctx); err != nil {
		t.Fatal(err)
	}
	xfer := resource.GetActiveTransfer()
	if xfer == nil {
		t.Fatal("expected active transfer")
	}
	return xfer.GetState()
}

// TestTransferRefusesSameAccount starts a Merge, Migrate and Mirror between two
// sessions of one account that holds no Space, and checks that each is refused
// and the account stays whole: a Merge or Migrate there would delete the
// target with the source.
func TestTransferRefusesSameAccount(t *testing.T) {
	// Create an account with two sessions and a session of another account.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	secondRef := srcRef.CloneVT()
	secondRef.GetProviderResourceRef().Id = ulid.NewULID()
	second, err := env.sessCtrl.RegisterSession(ctx, secondRef, &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   secondRef.GetProviderResourceRef().GetProviderAccountId(),
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRef, _ := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, otherRef)
	resource := env.buildSessionResource(ctx, t, otherRef)

	// Every mode is refused.
	for _, mode := range []provider_transfer.TransferMode{
		provider_transfer.TransferMode_TransferMode_MERGE,
		provider_transfer.TransferMode_TransferMode_MIGRATE,
		provider_transfer.TransferMode_TransferMode_MIRROR,
	} {
		_, err := resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
			SourceSessionIndex: srcIdx,
			TargetSessionIndex: second.GetSessionIndex(),
			Mode:               mode,
		})
		if err == nil || !strings.Contains(err.Error(), "same account") {
			t.Fatalf("%s between sessions of one account: expected a refusal, got %v", mode, err)
		}
	}

	// Both sessions stay listed.
	for _, idx := range []uint32{srcIdx, second.GetSessionIndex()} {
		entry, err := env.sessCtrl.GetSessionByIdx(ctx, idx)
		if err != nil {
			t.Fatal(err)
		}
		if entry == nil {
			t.Fatalf("session %d of the account is no longer listed", idx)
		}
	}

	// The account volume still mounts its session object store.
	provRef := srcRef.GetProviderResourceRef()
	objectStoreID := provider_local.SessionObjectStoreID(provRef.GetProviderId(), provRef.GetProviderAccountId())
	handle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, env.tb.Bus, false, objectStoreID, srcAcc.GetVolume().GetID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer diRef.Release()
	otx, err := handle.GetObjectStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	otx.Discard()
}
