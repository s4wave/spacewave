package resource_session_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_transfer "github.com/s4wave/spacewave/core/provider/transfer"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	"github.com/s4wave/spacewave/core/session"
	space_resolve "github.com/s4wave/spacewave/core/space/resolve"
	"github.com/s4wave/spacewave/db/volume"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
	"github.com/sirupsen/logrus"
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

// TestMergeLegacyDeleteKeepsSessions selects the legacy delete and checks that
// a Merge's source delete takes only the volume, as before the switch: the
// switch must restore the old behavior whole, not a mix of the two.
func TestMergeLegacyDeleteKeepsSessions(t *testing.T) {
	// Select the legacy delete for this test.
	defer resource_session.SetLegacyDelete(true)()

	// Merge a source account with one Space into a target.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "LegacySpace")
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef)
	resource := env.buildSessionResource(ctx, t, tgtRef)
	state := runTransfer(ctx, t, resource, srcIdx, tgtIdx, provider_transfer.TransferMode_TransferMode_MERGE)
	if state.GetPhase() != provider_transfer.TransferPhase_TransferPhase_COMPLETE {
		t.Fatalf("expected COMPLETE, got %s: %s", state.GetPhase(), state.GetErrorMessage())
	}

	// The source session stays listed, as the legacy delete left it.
	entry, err := env.sessCtrl.GetSessionByIdx(ctx, srcIdx)
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil {
		t.Fatal("the legacy delete removed the source session")
	}
}

// TestMergeLinkedSessionCompletesWhenUnlinkFails merges a local session
// linked to a cloud account that no loaded provider serves, and checks that
// the run completes with the source volume gone: the unlink follows the
// volume delete, so its failure leaves the entry UNLINKING for the resume.
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

	// The source session stays UNLINKING for the next startup, whose link the
	// unlink could not remove, and the target holds the Space.
	srcEntry, err := env.sessCtrl.GetSessionByIdx(ctx, srcIdx)
	if err != nil {
		t.Fatal(err)
	}
	if srcEntry.GetState() != session.SessionListState_SESSION_LIST_STATE_UNLINKING {
		t.Fatalf("source session after the Merge: %v; want UNLINKING", srcEntry)
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

// TestTransferRefusesResumeIntoReusedIndex checks that a start resuming an
// interrupted Merge by its session indexes is refused, and the source kept,
// when the target index now names another account than the one the run
// started with.
func TestTransferRefusesResumeIntoReusedIndex(t *testing.T) {
	// Create a source with one Space and a target that took a reused index.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)
	srcRef, srcIdx := env.createSession(ctx, t)
	srcAcc := env.accessAccount(ctx, t, srcRef)
	env.createSpaceOnAccount(ctx, t, srcAcc, "ResumeSpace")
	tgtRef, tgtIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, tgtRef)

	// Record an interrupted Merge at those indexes into an account since gone.
	provRef := srcRef.GetProviderResourceRef()
	checkpoint := provider_transfer.NewObjectStoreCheckpointLazy(
		env.tb.Bus,
		provider_local.SobjectObjectStoreID(provRef.GetProviderId(), provRef.GetProviderAccountId()),
		srcAcc.GetVolume().GetID(),
	)
	gone := tgtRef.GetProviderResourceRef().CloneVT()
	gone.Id, gone.ProviderAccountId = ulid.NewULID(), ulid.NewULID()
	err := checkpoint.SaveCheckpoint(ctx, &provider_transfer.TransferCheckpoint{
		State: &provider_transfer.TransferState{
			Mode:               provider_transfer.TransferMode_TransferMode_MERGE,
			Phase:              provider_transfer.TransferPhase_TransferPhase_COPYING_BLOCKS,
			SourceSessionIndex: srcIdx,
			TargetSessionIndex: tgtIdx,
		},
		SourceSession: provRef.CloneVT(),
		TargetSession: gone,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The resume is refused, drops the checkpoint and keeps the source.
	resource := env.buildSessionResource(ctx, t, tgtRef)
	_, err = resource.StartTransfer(ctx, &s4wave_session.StartTransferRequest{
		SourceSessionIndex: srcIdx,
		TargetSessionIndex: tgtIdx,
		Mode:               provider_transfer.TransferMode_TransferMode_MERGE,
	})
	if err == nil || !strings.Contains(err.Error(), "sessions changed") {
		t.Fatalf("resume into a reused index: expected a refusal, got %v", err)
	}
	if cp, err := checkpoint.LoadCheckpoint(ctx); err != nil || cp != nil {
		t.Fatalf("checkpoint after the refusal = %v, %v; want none", cp, err)
	}
	if entry, err := env.sessCtrl.GetSessionByIdx(ctx, srcIdx); err != nil || entry == nil {
		t.Fatalf("source session after the refusal = %v, %v; want listed", entry, err)
	}
}

// deletingEnv is an account with a Space and two sessions, and an unrelated
// account, for the tests of a stopped delete.
type deletingEnv struct {
	*testEnv
	ref, second         *session.SessionRef
	idx, secondIdx      uint32
	otherIdx            uint32
	acc                 *provider_local.ProviderAccount
	accountID, objStore string
}

// setupDeletingEnv builds a deletingEnv whose account holds a Space named
// spaceName.
func setupDeletingEnv(ctx context.Context, t *testing.T, spaceName string) *deletingEnv {
	t.Helper()
	env := setupTestEnv(ctx, t)
	ref, idx := env.createSession(ctx, t)
	acc := env.accessAccount(ctx, t, ref)
	env.createSpaceOnAccount(ctx, t, acc, spaceName)
	second := ref.CloneVT()
	second.GetProviderResourceRef().Id = ulid.NewULID()
	entry, err := env.sessCtrl.RegisterSession(ctx, second, &session.SessionMetadata{
		ProviderDisplayName: "Local",
		ProviderId:          "local",
		ProviderAccountId:   second.GetProviderResourceRef().GetProviderAccountId(),
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRef, otherIdx := env.createSession(ctx, t)
	_ = env.accessAccount(ctx, t, otherRef)
	provRef := ref.GetProviderResourceRef()
	return &deletingEnv{
		testEnv:   env,
		ref:       ref,
		second:    second,
		idx:       idx,
		secondIdx: entry.GetSessionIndex(),
		otherIdx:  otherIdx,
		acc:       acc,
		accountID: provRef.GetProviderAccountId(),
		objStore:  provider_local.SessionObjectStoreID(provRef.GetProviderId(), provRef.GetProviderAccountId()),
	}
}

// deleteStoppedAfter runs the account's delete and stops it after step.
func (e *deletingEnv) deleteStoppedAfter(ctx context.Context, t *testing.T, step resource_session.DeleteStep) {
	t.Helper()
	restore := resource_session.StopAfter(step)
	defer restore()
	le := logrus.NewEntry(logrus.New())
	err := resource_session.DeleteProviderAccountWithin(ctx, le, e.tb.Bus, e.sessCtrl, "local", e.accountID, e.objStore, e.acc.GetVolume(), time.Minute)
	if !errors.Is(err, resource_session.ErrStopped) {
		t.Fatalf("expected the delete stopped, got %v", err)
	}
}

// requireState checks that both of the account's entries are listed in state.
func (e *deletingEnv) requireState(ctx context.Context, t *testing.T, state session.SessionListState) {
	t.Helper()
	for _, idx := range []uint32{e.idx, e.secondIdx} {
		entry, err := e.sessCtrl.GetSessionByIdx(ctx, idx)
		if err != nil {
			t.Fatal(err)
		}
		if entry.GetState() != state {
			t.Fatalf("session %d: state %s, want %s", idx, entry.GetState(), state)
		}
	}
}

// resume runs the startup resume and checks that it removed both of the
// account's entries and left the unrelated account's.
func (e *deletingEnv) resume(ctx context.Context, t *testing.T) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	if err := resource_session.ResumeAccountDeletes(ctx, le, e.tb.Bus); err != nil {
		t.Fatal(err)
	}
	for _, idx := range []uint32{e.idx, e.secondIdx} {
		entry, err := e.sessCtrl.GetSessionByIdx(ctx, idx)
		if err != nil {
			t.Fatal(err)
		}
		if entry != nil {
			t.Fatalf("session %d of the deleted account still listed", idx)
		}
	}
	other, err := e.sessCtrl.GetSessionByIdx(ctx, e.otherIdx)
	if err != nil {
		t.Fatal(err)
	}
	if other == nil {
		t.Fatal("the resume removed an unrelated account's session")
	}
}

// requireMountRefused checks that a mount of ref is refused as deleting.
func requireMountRefused(ctx context.Context, t *testing.T, env *testEnv, ref *session.SessionRef) {
	t.Helper()
	mctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, sessRef, err := session.ExMountSession(mctx, env.tb.Bus, ref, false, nil)
	if err == nil {
		sessRef.Release()
		t.Fatalf("session %s of a deleting account mounted", ref.GetProviderResourceRef().GetId())
	}
	if !errors.Is(err, session.ErrAccountDeleting) {
		t.Fatalf("expected ErrAccountDeleting, got %v", err)
	}
}

// countSessionKeys counts the keys of ref's session in the account's session
// object store.
func (e *deletingEnv) countSessionKeys(ctx context.Context, t *testing.T, ref *session.SessionRef) int {
	t.Helper()
	handle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, e.tb.Bus, false, e.objStore, e.acc.GetVolume().GetID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer diRef.Release()
	otx, err := handle.GetObjectStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer otx.Discard()
	n := 0
	prefix := []byte(ref.GetProviderResourceRef().GetId() + "/")
	err = otx.ScanPrefixKeys(ctx, prefix, func([]byte) error {
		n++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// TestMountResumesDeletingAccount stops a delete after its mark, as a crash
// there leaves it, and checks that a mount of the second session is refused
// and the startup resume removes both sessions while an unrelated account's
// session stays listed.
func TestMountResumesDeletingAccount(t *testing.T) {
	// Stop the account's delete after the mark.
	ctx := context.Background()
	env := setupDeletingEnv(ctx, t, "DeletingSpace")
	env.deleteStoppedAfter(ctx, t, resource_session.StepMarked)
	env.requireState(ctx, t, session.SessionListState_SESSION_LIST_STATE_DELETING)

	// The second session does not mount, and the resume finishes the delete.
	requireMountRefused(ctx, t, env.testEnv, env.second)
	env.resume(ctx, t)
}

// TestResumeAfterKeysMintsNoKey stops a delete after it removed the session
// keys, as a crash there leaves it, and checks that no mount mints a key for
// either session and the resume completes the delete.
func TestResumeAfterKeysMintsNoKey(t *testing.T) {
	// Stop the account's delete after the keys.
	ctx := context.Background()
	env := setupDeletingEnv(ctx, t, "KeysSpace")
	if n := env.countSessionKeys(ctx, t, env.ref); n == 0 {
		t.Fatal("the session has no key before the delete")
	}
	env.deleteStoppedAfter(ctx, t, resource_session.StepKeys)
	env.requireState(ctx, t, session.SessionListState_SESSION_LIST_STATE_DELETING)

	// Mounts of both sessions are refused and mint no key.
	for _, ref := range []*session.SessionRef{env.ref, env.second} {
		requireMountRefused(ctx, t, env.testEnv, ref)
		if n := env.countSessionKeys(ctx, t, ref); n != 0 {
			t.Fatalf("session %s has %d keys after the mount", ref.GetProviderResourceRef().GetId(), n)
		}
	}

	// The resume completes the delete: the entries and the volume are gone.
	before := env.storage.ListVolumes()
	env.resume(ctx, t)
	after := env.storage.ListVolumes()
	gone := slices.DeleteFunc(slices.Clone(before), func(id string) bool { return slices.Contains(after, id) })
	if len(gone) != 1 || len(after) != len(before)-1 {
		t.Fatalf("volumes %v after the resume, from %v; want only the account's removed", after, before)
	}
}

// TestResumeAfterVolumeCreatesNoVolume stops a delete after it removed the
// volume, as a crash there leaves it, and checks that each way a walker of
// the session list mounts a session of the account is refused and the resume
// finishes, without either creating a volume: past the volume delete no mark
// remains on disk, so only the list refuses.
func TestResumeAfterVolumeCreatesNoVolume(t *testing.T) {
	// Stop the account's delete after the volume.
	ctx := context.Background()
	env := setupDeletingEnv(ctx, t, "VolumeSpace")
	env.deleteStoppedAfter(ctx, t, resource_session.StepVolume)
	env.requireState(ctx, t, session.SessionListState_SESSION_LIST_STATE_UNLINKING)
	before := env.storage.ListVolumes()

	// The Space resolver mounts by index.
	if _, rel, err := space_resolve.ResolveSpace(ctx, env.tb.Bus, env.idx, "volumespace-id"); err == nil {
		rel()
		t.Fatal("the Space of a deleted account resolved")
	} else if !errors.Is(err, session.ErrAccountDeleting) {
		t.Fatalf("resolve: expected ErrAccountDeleting, got %v", err)
	}

	// The status projector mounts the listed ref, the background holds a ref
	// built from its ids, and a session of the account the list does not
	// name is refused as well.
	listed, err := env.sessCtrl.GetSessionByIdx(ctx, env.secondIdx)
	if err != nil {
		t.Fatal(err)
	}
	provRef := env.ref.GetProviderResourceRef()
	built := &session.SessionRef{ProviderResourceRef: &provider.ProviderResourceRef{
		ProviderId:        provRef.GetProviderId(),
		ProviderAccountId: provRef.GetProviderAccountId(),
		Id:                provRef.GetId(),
	}}
	unlisted := env.ref.CloneVT()
	unlisted.GetProviderResourceRef().Id = ulid.NewULID()
	for _, ref := range []*session.SessionRef{listed.GetSessionRef(), built, unlisted} {
		requireMountRefused(ctx, t, env.testEnv, ref)
	}
	if got := env.storage.ListVolumes(); !slices.Equal(got, before) {
		t.Fatalf("volumes %v after the mounts, from %v", got, before)
	}

	// The resume removes the entries and creates no volume.
	env.resume(ctx, t)
	if got := env.storage.ListVolumes(); !slices.Equal(got, before) {
		t.Fatalf("volumes %v after the resume, from %v", got, before)
	}
}

// hungSessionController is a session controller whose SetSessionState writes
// the states and then never returns before its context ends, as a session
// whose lifetime never stops.
type hungSessionController struct {
	session.SessionController
}

// SetSessionState writes the states and waits for ctx to end.
func (c hungSessionController) SetSessionState(ctx context.Context, refs []*session.SessionRef, state session.SessionListState) error {
	if err := c.SessionController.SetSessionState(ctx, refs, state); err != nil {
		return err
	}
	<-ctx.Done()
	return ctx.Err()
}

// TestDeleteKeepsVolumeWhenSessionHangs runs an account delete whose session
// delete never returns, and checks that the delete fails within its bound and
// the volume and the session stay: a delete that went on to the volume would
// leave the session listed over nothing.
func TestDeleteKeepsVolumeWhenSessionHangs(t *testing.T) {
	// Create an account with a Space.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)
	ref, idx := env.createSession(ctx, t)
	acc := env.accessAccount(ctx, t, ref)
	env.createSpaceOnAccount(ctx, t, acc, "HungSpace")

	// Delete it through a controller whose session delete hangs.
	provRef := ref.GetProviderResourceRef()
	providerID, accountID := provRef.GetProviderId(), provRef.GetProviderAccountId()
	objectStoreID := provider_local.SessionObjectStoreID(providerID, accountID)
	le := logrus.NewEntry(logrus.New())
	start := time.Now()
	err := resource_session.DeleteProviderAccountWithin(ctx, le, env.tb.Bus, hungSessionController{env.sessCtrl}, providerID, accountID, objectStoreID, acc.GetVolume(), 2*time.Second)
	if err == nil {
		t.Fatal("the delete finished with a session that never stopped")
	}
	if took := time.Since(start); took > 20*time.Second {
		t.Fatalf("the delete took %s past its 2s bound", took)
	}

	// The session stays listed and its account still lists the Space.
	entry, err := env.sessCtrl.GetSessionByIdx(ctx, idx)
	if err != nil {
		t.Fatal(err)
	}
	if entry == nil {
		t.Fatal("the session was removed")
	}
	if n := countSpaces(t, acc); n != 1 {
		t.Fatalf("expected the Space kept, found %d", n)
	}
}
