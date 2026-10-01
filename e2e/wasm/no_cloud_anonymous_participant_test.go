//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestNoCloudAnonymousParticipantSync covers Phase 15.6 of the account-
// lifecycle scope: an anonymous P2P-only participant added via direct
// pairing receives SharedObject state without a cloud relay. Both peers
// run on local providers (no spacewave provider, no cloud account); the
// only transport between them is the WebRTC link established by the no-
// cloud pair-code flow exercised in TestNoCloudPairingDirect.
//
// Flow:
//  1. Two browser sessions each create a fresh local provider account.
//  2. They pair via CreateLocalPairingOffer / AcceptLocalPairingOffer /
//     AcceptLocalPairingAnswer (same as TestNoCloudPairingDirect).
//  3. Both sides confirm the SAS match and ConfirmPairing, which adds the
//     remote peer as OWNER on every existing and future SharedObject.
//  4. The owner (A) creates a Space via CreateSpace.
//  5. The participant (B) observes the Space appear in WatchResourcesList
//     within a bounded timeout, proving SO list state synced peer-to-peer
//     over the bifrost link with no cloud provider involved.
func TestNoCloudAnonymousParticipantSync(t *testing.T) {
	// Skip the P2P sync journey until no-cloud resource-list discovery is wired.
	t.Skip("no-cloud resource-list discovery is not wired yet; direct pairing coverage lives in TestNoCloudPairingDirect")

	// Create two fresh browser sessions backed by local provider accounts.
	sessA := harness(t).NewCleanSession(t)
	sessB := harness(t).NewCleanSession(t)
	ctx, cancel := context.WithTimeout(harness(t).Context(), 3*time.Minute)
	t.Cleanup(cancel)

	// Mount the SDK session for each browser session.
	sdkA := mountFreshLocalSession(ctx, t, sessA)
	defer sdkA.Release()
	sdkB := mountFreshLocalSession(ctx, t, sessB)
	defer sdkB.Release()

	// Open pairing status watches on both sides.
	watchA, err := sdkA.WatchPairingStatus(ctx)
	if err != nil {
		t.Fatalf("WatchPairingStatus A: %v", err)
	}
	defer watchA.Close()
	watchB, err := sdkB.WatchPairingStatus(ctx)
	if err != nil {
		t.Fatalf("WatchPairingStatus B: %v", err)
	}
	defer watchB.Close()

	// Both sides must start in the idle pairing state.
	expectInitialPairingStatus(t, "A", watchA, s4wave_session.PairingStatus_PairingStatus_IDLE)
	expectInitialPairingStatus(t, "B", watchB, s4wave_session.PairingStatus_PairingStatus_IDLE)

	// Exchange the local pairing offer, answer, and final answer.
	offerResp, err := sdkA.CreateLocalPairingOffer(ctx)
	if err != nil {
		t.Fatalf("CreateLocalPairingOffer (A): %v", err)
	}
	answerResp, err := sdkB.AcceptLocalPairingOffer(ctx, offerResp.GetOfferPayload(), false)
	if err != nil {
		t.Fatalf("AcceptLocalPairingOffer (B): %v", err)
	}
	finalAnswerResp, err := sdkA.AcceptLocalPairingAnswer(ctx, answerResp.GetAnswerPayload())
	if err != nil {
		t.Fatalf("AcceptLocalPairingAnswer (A): %v", err)
	}
	remotePeerOnA := finalAnswerResp.GetRemotePeerId()
	if remotePeerOnA == "" {
		t.Fatal("AcceptLocalPairingAnswer returned empty remote peer id on A")
	}

	// VERIFYING_EMOJI implies the link reached PEER_CONNECTED. Waiting for the
	// durable phase avoids requiring the snapshot stream to expose both states.
	waitForPairingStatus(t, "A", watchA, s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI)
	remotePeerOnB := waitForPairingStatusRemotePeer(
		t, "B", watchB, s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI,
	)
	if remotePeerOnB == "" {
		t.Fatal("expected B to learn remote peer ID during pairing verification")
	}

	// Fetch the SAS emoji code from both sides.
	emojiA, err := sdkA.GetSASEmoji(ctx, remotePeerOnA)
	if err != nil {
		t.Fatalf("GetSASEmoji (A): %v", err)
	}
	emojiB, err := sdkB.GetSASEmoji(ctx, remotePeerOnB)
	if err != nil {
		t.Fatalf("GetSASEmoji (B): %v", err)
	}

	// Both sides must display the identical non-empty emoji sequence.
	if len(emojiA.GetEmoji()) == 0 || len(emojiB.GetEmoji()) == 0 {
		t.Fatalf("SAS emoji empty: A=%v B=%v", emojiA.GetEmoji(), emojiB.GetEmoji())
	}
	if !equalStringSlices(emojiA.GetEmoji(), emojiB.GetEmoji()) {
		t.Fatalf("SAS emoji mismatch: A=%v B=%v", emojiA.GetEmoji(), emojiB.GetEmoji())
	}

	// Confirm the SAS match on both sides.
	if err := sdkA.ConfirmSASMatch(ctx, true); err != nil {
		t.Fatalf("ConfirmSASMatch (A): %v", err)
	}
	if err := sdkB.ConfirmSASMatch(ctx, true); err != nil {
		t.Fatalf("ConfirmSASMatch (B): %v", err)
	}

	// Confirm the pairing from both peers concurrently.
	confirmPairingBothSides(ctx, t, sdkA, remotePeerOnA, "device-b", sdkB, remotePeerOnB, "device-a")

	// Both sides must reach the fully confirmed pairing state.
	waitForPairingStatus(t, "A", watchA, s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED)
	waitForPairingStatus(t, "B", watchB, s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED)

	// Create a Space owned by peer A.
	spaceName := "P2P Sync Space"
	createResp, err := sdkA.CreateSpace(ctx, &s4wave_session.CreateSpaceRequest{SpaceName: spaceName})
	if err != nil {
		t.Fatalf("CreateSpace on A: %v", err)
	}
	spaceID := createResp.GetSharedObjectRef().GetProviderResourceRef().GetId()
	if spaceID == "" {
		t.Fatal("CreateSpace returned empty shared object id")
	}
	t.Logf("owner A created space %s", spaceID)

	// Participant B must observe the Space arrive over the P2P link.
	if err := waitForSpaceInResourcesList(ctx, sdkB, spaceID); err != nil {
		t.Fatalf("participant B did not observe space %s over P2P: %v", spaceID, err)
	}
}

// confirmPairingBothSides confirms the pairing from both peers concurrently and
// fails the test if either side errors.
func confirmPairingBothSides(
	ctx context.Context,
	t *testing.T,
	sdkA *s4wave_session.Session,
	remotePeerOnA string,
	deviceNameA string,
	sdkB *s4wave_session.Session,
	remotePeerOnB string,
	deviceNameB string,
) {
	// Mark the pairing helper as a test helper.
	t.Helper()

	// Run both ConfirmPairing calls in parallel goroutines.
	var wg sync.WaitGroup
	var errA error
	var errB error
	wg.Add(2)
	go func() {
		defer wg.Done()
		errA = sdkA.ConfirmPairing(ctx, remotePeerOnA, deviceNameA)
	}()
	go func() {
		defer wg.Done()
		errB = sdkB.ConfirmPairing(ctx, remotePeerOnB, deviceNameB)
	}()
	wg.Wait()

	// Fail the test if either side failed to confirm.
	if errA != nil {
		t.Fatalf("ConfirmPairing (A): %v", errA)
	}
	if errB != nil {
		t.Fatalf("ConfirmPairing (B): %v", errB)
	}
}

// waitForSpaceInResourcesList consumes WatchResourcesList until the given
// shared object ID appears in a snapshot or the context expires.
func waitForSpaceInResourcesList(
	ctx context.Context,
	sdk *s4wave_session.Session,
	spaceID string,
) error {
	// Open the resources list watch stream.
	stream, err := sdk.WatchResourcesList(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

	// Consume snapshots until the requested space ID appears.
	for {
		resp, err := stream.Recv()
		if err != nil {
			return err
		}
		for _, entry := range resp.GetSpacesList() {
			id := entry.GetEntry().GetRef().GetProviderResourceRef().GetId()
			if id == spaceID {
				return nil
			}
		}
	}
}

// waitForPairingStatusRemotePeer blocks until a snapshot reaches the requested
// successful phase, then returns its RemotePeerId. It fails on stream error or
// a terminal pairing state.
func waitForPairingStatusRemotePeer(
	t *testing.T,
	side string,
	stream s4wave_session.SRPCSessionResourceService_WatchPairingStatusClient,
	want s4wave_session.PairingStatus,
) string {
	t.Helper()
	// Consume snapshots until the requested phase or a terminal failure arrives.
	for {
		resp, err := stream.Recv()
		if err != nil {
			t.Fatalf("WatchPairingStatus %s recv: %v", side, err)
		}
		status := resp.GetStatus()
		t.Logf("pairing status %s: %s remote=%s", side, status.String(), resp.GetRemotePeerId())
		if pairingStatusReached(status, want) {
			return resp.GetRemotePeerId()
		}
		switch status {
		case s4wave_session.PairingStatus_PairingStatus_FAILED,
			s4wave_session.PairingStatus_PairingStatus_SIGNALING_FAILED,
			s4wave_session.PairingStatus_PairingStatus_CONNECTION_TIMEOUT,
			s4wave_session.PairingStatus_PairingStatus_PAIRING_REJECTED,
			s4wave_session.PairingStatus_PairingStatus_CONFIRMATION_TIMEOUT:
			t.Fatalf(
				"pairing %s reached error state %s before %s (msg=%q)",
				side,
				status.String(),
				want.String(),
				resp.GetErrorMessage(),
			)
		}
	}
}

// equalStringSlices reports whether the two slices match case-insensitively.
func equalStringSlices(a, b []string) bool {
	// Compare each element case-insensitively after checking lengths.
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !strings.EqualFold(a[i], b[i]) {
			return false
		}
	}
	return true
}
