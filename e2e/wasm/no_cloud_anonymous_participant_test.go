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
	t.Skip("no-cloud resource-list discovery is not wired yet; direct pairing coverage lives in TestNoCloudPairingDirect")

	sessA := testHarness.NewSession(t)
	sessB := testHarness.NewSession(t)

	ctx, cancel := context.WithTimeout(testHarness.Context(), 3*time.Minute)
	t.Cleanup(cancel)

	sdkA := mountFreshLocalSession(ctx, t, sessA)
	defer sdkA.Release()
	sdkB := mountFreshLocalSession(ctx, t, sessB)
	defer sdkB.Release()

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

	drainPairingStatusUntil(watchA, s4wave_session.PairingStatus_PairingStatus_IDLE)
	drainPairingStatusUntil(watchB, s4wave_session.PairingStatus_PairingStatus_IDLE)

	offerResp, err := sdkA.CreateLocalPairingOffer(ctx)
	if err != nil {
		t.Fatalf("CreateLocalPairingOffer (A): %v", err)
	}
	answerResp, err := sdkB.AcceptLocalPairingOffer(ctx, offerResp.GetOfferPayload())
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

	// A already saw the PEER_CONNECTED transition implicitly via the answer
	// exchange; consume its watch stream up to that point so subsequent waits
	// observe BOTH_CONFIRMED rather than the prior emission.
	waitForPairingStatus(t, "A", watchA, s4wave_session.PairingStatus_PairingStatus_PEER_CONNECTED)

	remotePeerOnB := waitForPairingStatusRemotePeer(
		t, "B", watchB, s4wave_session.PairingStatus_PairingStatus_PEER_CONNECTED,
	)
	if remotePeerOnB == "" {
		t.Fatal("expected B to learn remote peer ID via PEER_CONNECTED status")
	}
	waitForPairingStatus(t, "A", watchA, s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI)
	waitForPairingStatus(t, "B", watchB, s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI)

	emojiA, err := sdkA.GetSASEmoji(ctx, remotePeerOnA)
	if err != nil {
		t.Fatalf("GetSASEmoji (A): %v", err)
	}
	emojiB, err := sdkB.GetSASEmoji(ctx, remotePeerOnB)
	if err != nil {
		t.Fatalf("GetSASEmoji (B): %v", err)
	}
	if len(emojiA.GetEmoji()) == 0 || len(emojiB.GetEmoji()) == 0 {
		t.Fatalf("SAS emoji empty: A=%v B=%v", emojiA.GetEmoji(), emojiB.GetEmoji())
	}
	if !equalStringSlices(emojiA.GetEmoji(), emojiB.GetEmoji()) {
		t.Fatalf("SAS emoji mismatch: A=%v B=%v", emojiA.GetEmoji(), emojiB.GetEmoji())
	}

	if err := sdkA.ConfirmSASMatch(ctx, true); err != nil {
		t.Fatalf("ConfirmSASMatch (A): %v", err)
	}
	if err := sdkB.ConfirmSASMatch(ctx, true); err != nil {
		t.Fatalf("ConfirmSASMatch (B): %v", err)
	}

	confirmPairingBothSides(ctx, t, sdkA, remotePeerOnA, "device-b", sdkB, remotePeerOnB, "device-a")

	waitForPairingStatus(t, "A", watchA, s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED)
	waitForPairingStatus(t, "B", watchB, s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED)

	spaceName := "P2P Sync Space"
	createResp, err := sdkA.CreateSpace(ctx, spaceName, "", "")
	if err != nil {
		t.Fatalf("CreateSpace on A: %v", err)
	}
	spaceID := createResp.GetSharedObjectRef().GetProviderResourceRef().GetId()
	if spaceID == "" {
		t.Fatal("CreateSpace returned empty shared object id")
	}
	t.Logf("owner A created space %s", spaceID)

	if err := waitForSpaceInResourcesList(ctx, sdkB, spaceID); err != nil {
		t.Fatalf("participant B did not observe space %s over P2P: %v", spaceID, err)
	}
}

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
	t.Helper()

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
	stream, err := sdk.WatchResourcesList(ctx)
	if err != nil {
		return err
	}
	defer stream.Close()

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

// waitForPairingStatusRemotePeer blocks until the stream emits a response whose
// status matches want, then returns the RemotePeerId from that response. Fails
// the test on timeout, stream error, or terminal pairing failure.
func waitForPairingStatusRemotePeer(
	t *testing.T,
	side string,
	stream s4wave_session.SRPCSessionResourceService_WatchPairingStatusClient,
	want s4wave_session.PairingStatus,
) string {
	t.Helper()
	for {
		resp, err := stream.Recv()
		if err != nil {
			t.Fatalf("WatchPairingStatus %s recv: %v", side, err)
		}
		t.Logf("pairing status %s: %s remote=%s", side, resp.GetStatus().String(), resp.GetRemotePeerId())
		switch resp.GetStatus() {
		case want:
			return resp.GetRemotePeerId()
		case s4wave_session.PairingStatus_PairingStatus_FAILED,
			s4wave_session.PairingStatus_PairingStatus_SIGNALING_FAILED,
			s4wave_session.PairingStatus_PairingStatus_CONNECTION_TIMEOUT,
			s4wave_session.PairingStatus_PairingStatus_PAIRING_REJECTED,
			s4wave_session.PairingStatus_PairingStatus_CONFIRMATION_TIMEOUT:
			t.Fatalf(
				"pairing %s reached error state %s before %s (msg=%q)",
				side,
				resp.GetStatus().String(),
				want.String(),
				resp.GetErrorMessage(),
			)
		}
	}
}

func equalStringSlices(a, b []string) bool {
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
