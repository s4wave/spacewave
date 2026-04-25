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
// Success criterion: both sides observe PairingStatus_PEER_CONNECTED, which
// is set by ProviderAccount.OnDirectPairingConnected once the WebRTC data
// channel is open and the bifrost link is wired up.
func TestNoCloudPairingDirect(t *testing.T) {
	sessA := testHarness.NewSession(t)
	sessB := testHarness.NewSession(t)

	ctx, cancel := context.WithTimeout(testHarness.Context(), 90*time.Second)
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

	// Drain the initial IDLE emission so the test only observes transitions
	// caused by the offer/answer exchange.
	drainPairingStatusUntil(watchA, s4wave_session.PairingStatus_PairingStatus_IDLE)
	drainPairingStatusUntil(watchB, s4wave_session.PairingStatus_PairingStatus_IDLE)

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

// drainPairingStatusUntil consumes pairing status messages non-blocking until
// one matching want is observed or the channel has no further pending
// messages. Used to skip the initial IDLE emission before asserting on a
// state transition.
func drainPairingStatusUntil(
	stream s4wave_session.SRPCSessionResourceService_WatchPairingStatusClient,
	want s4wave_session.PairingStatus,
) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	done := make(chan struct{})
	var resp *s4wave_session.WatchPairingStatusResponse
	go func() {
		defer close(done)
		for {
			r, err := stream.Recv()
			if err != nil {
				return
			}
			resp = r
			if resp.GetStatus() == want {
				return
			}
		}
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}
}

// waitForPairingStatus blocks until the stream emits a response whose status
// matches want, then returns. Fails the test on timeout or stream error.
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
		t.Logf("pairing status %s: %s", side, resp.GetStatus().String())
		switch resp.GetStatus() {
		case want:
			return
		case s4wave_session.PairingStatus_PairingStatus_FAILED,
			s4wave_session.PairingStatus_PairingStatus_SIGNALING_FAILED,
			s4wave_session.PairingStatus_PairingStatus_CONNECTION_TIMEOUT,
			s4wave_session.PairingStatus_PairingStatus_PAIRING_REJECTED:
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
