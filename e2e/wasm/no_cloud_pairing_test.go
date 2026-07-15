//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"testing"
	"time"

	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestNoCloudPairingDirect verifies the no-cloud pairing RPCs (CreateLocal-
// PairingOffer / AcceptLocalPairingOffer / AcceptLocalPairingAnswer) drive a
// real WebRTC link between two isolated browser sessions running in the same
// Playwright harness with --allow-loopback-in-peer-connection.
//
// Each session creates an independent local provider account, mounts its
// session, opens a WatchPairingStatus stream, and then exchanges the SDP
// offer/answer payloads through the Go test process (standing in for the QR
// or paste-based out-of-band channel the UI normally uses).
//
// Success criterion: both sides observe PEER_CONNECTED or a later successful
// pairing phase, which proves the WebRTC data channel opened and bifrost link
// wiring completed even when the snapshot stream coalesces a transition.
func TestNoCloudPairingDirect(t *testing.T) {
	compiler, err := ResolveE2EWasmCompiler()
	if err != nil {
		t.Fatalf("resolve wasm compiler: %v", err)
	}
	if !e2eWasmBrowserWebRTCEnabled(compiler) {
		t.Skipf("requires browser WebRTC transport support; compiler=%s", compiler)
	}

	sessA := harness(t).NewCleanSession(t)
	sessB := harness(t).NewCleanSession(t)

	ctx, cancel := context.WithTimeout(harness(t).Context(), 90*time.Second)
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

	// Consume the initial IDLE snapshot so later reads only observe transitions
	// caused by the offer/answer exchange.
	expectInitialPairingStatus(t, "A", watchA, s4wave_session.PairingStatus_PairingStatus_IDLE)
	expectInitialPairingStatus(t, "B", watchB, s4wave_session.PairingStatus_PairingStatus_IDLE)

	offerResp, err := sdkA.CreateLocalPairingOffer(ctx)
	if err != nil {
		t.Fatalf("CreateLocalPairingOffer (A): %v", err)
	}
	if offerResp.GetOfferPayload() == "" {
		t.Fatal("expected non-empty offer payload from A")
	}

	answerResp, err := sdkB.AcceptLocalPairingOffer(ctx, offerResp.GetOfferPayload())
	if err != nil {
		t.Fatalf("AcceptLocalPairingOffer (B): %v", err)
	}
	if answerResp.GetAnswerPayload() == "" {
		t.Fatal("expected non-empty answer payload from B")
	}

	finalAnswerResp, err := sdkA.AcceptLocalPairingAnswer(ctx, answerResp.GetAnswerPayload())
	if err != nil {
		t.Fatalf("AcceptLocalPairingAnswer (A): %v", err)
	}
	if finalAnswerResp.GetRemotePeerId() == "" {
		t.Fatal("expected non-empty remote peer ID from A's AcceptLocalPairingAnswer")
	}

	waitForPairingStatus(t, "A", watchA, s4wave_session.PairingStatus_PairingStatus_PEER_CONNECTED)
	waitForPairingStatus(t, "B", watchB, s4wave_session.PairingStatus_PairingStatus_PEER_CONNECTED)
}

// mountFreshLocalSession creates a brand-new local provider account on the
// session and mounts the resulting session resource. The returned SDK Session
// must be released by the caller.
func mountFreshLocalSession(ctx context.Context, t *testing.T, sess *TestSession) *s4wave_session.Session {
	t.Helper()

	root := sess.Root()
	if root == nil {
		t.Fatal("expected non-nil root resource")
	}

	provID, err := root.LookupProvider(ctx, "local")
	if err != nil {
		t.Fatalf("LookupProvider local: %v", err)
	}
	provRef := sess.ResourceClient().CreateResourceReference(provID)
	defer provRef.Release()

	lp, err := s4wave_provider_local.NewLocalProvider(sess.ResourceClient(), provRef)
	if err != nil {
		t.Fatalf("NewLocalProvider: %v", err)
	}

	resp, err := lp.CreateAccount(ctx)
	if err != nil {
		t.Fatalf("CreateAccount on local provider: %v", err)
	}
	idx := resp.GetSessionListEntry().GetSessionIndex()
	if idx == 0 {
		t.Fatal("expected non-zero session index from CreateAccount")
	}

	sdk, err := sess.MountSessionByIdx(ctx, idx)
	if err != nil {
		t.Fatalf("MountSessionByIdx %d: %v", idx, err)
	}
	return sdk
}

// expectInitialPairingStatus consumes and validates the watch's initial snapshot.
func expectInitialPairingStatus(
	t *testing.T,
	side string,
	stream s4wave_session.SRPCSessionResourceService_WatchPairingStatusClient,
	want s4wave_session.PairingStatus,
) {
	t.Helper()
	resp, err := stream.Recv()
	if err != nil {
		t.Fatalf("initial WatchPairingStatus %s recv: %v", side, err)
	}
	if got := resp.GetStatus(); got != want {
		t.Fatalf(
			"initial pairing status %s = %s, want %s",
			side,
			got.String(),
			want.String(),
		)
	}
}

// pairingStatusReached reports whether got proves the requested successful phase.
func pairingStatusReached(got, want s4wave_session.PairingStatus) bool {
	if got == want {
		return true
	}
	switch want {
	case s4wave_session.PairingStatus_PairingStatus_PEER_CONNECTED:
		switch got {
		case s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI,
			s4wave_session.PairingStatus_PairingStatus_VERIFIED,
			s4wave_session.PairingStatus_PairingStatus_WAITING_FOR_REMOTE_CONFIRM,
			s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED:
			return true
		}
	case s4wave_session.PairingStatus_PairingStatus_VERIFYING_EMOJI:
		switch got {
		case s4wave_session.PairingStatus_PairingStatus_VERIFIED,
			s4wave_session.PairingStatus_PairingStatus_WAITING_FOR_REMOTE_CONFIRM,
			s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED:
			return true
		}
	case s4wave_session.PairingStatus_PairingStatus_VERIFIED:
		switch got {
		case s4wave_session.PairingStatus_PairingStatus_WAITING_FOR_REMOTE_CONFIRM,
			s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED:
			return true
		}
	case s4wave_session.PairingStatus_PairingStatus_WAITING_FOR_REMOTE_CONFIRM:
		return got == s4wave_session.PairingStatus_PairingStatus_BOTH_CONFIRMED
	}
	return false
}

// waitForPairingStatus blocks until a snapshot reaches the requested successful
// phase, then returns. Fails the test on stream error or terminal pairing state.
func waitForPairingStatus(
	t *testing.T,
	side string,
	stream s4wave_session.SRPCSessionResourceService_WatchPairingStatusClient,
	want s4wave_session.PairingStatus,
) {
	t.Helper()
	for {
		resp, err := stream.Recv()
		if err != nil {
			t.Fatalf("WatchPairingStatus %s recv: %v", side, err)
		}
		status := resp.GetStatus()
		t.Logf("pairing status %s: %s", side, status.String())
		if pairingStatusReached(status, want) {
			return
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
