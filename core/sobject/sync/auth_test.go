package sobject_sync

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestParticipantAuthenticationRequiresExplicitAdmission rejects a different
// message after a valid proof without reporting it as a remote access decision.
func TestParticipantAuthenticationRequiresExplicitAdmission(t *testing.T) {

	// withTimeout ctx,cancel via context.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	const soID = "authentication-admission"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	local := newAuthenticationPeer(t, soID, owner, authenticationState(t, soID, owner, reader))
	admissions := make(chan bool, 1)

	// func.
	local.peerAdmission = func(_ peer.ID, accepted bool) { admissions <- accepted }
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	done := make(chan error, 1)
	go func() { done <- local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	remote := stream_packet.NewSession(right, 64*1024)

	// Prove possession on the actual challenged transport before sending the wrong message.
	nonce := bytes.Repeat([]byte{1}, authenticationNonceSize)
	challenge, err := exchangeMessage(remote, false, &SOSyncMessage{Body: &SOSyncMessage_Challenge{Challenge: &SOSyncChallenge{Nonce: nonce}}})
	if err != nil {
		t.Fatal(err)
	}
	transcript := &SOSyncAuthTranscript{
		SharedObjectId: soID, SenderTransport: []byte("transport-b"), ReceiverTransport: []byte("transport-a"),
		SenderNonce: nonce, ReceiverNonce: challenge.GetChallenge().GetNonce(),
	}
	data, err := transcript.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	proof, err := peer.NewSignature(authenticationContext, reader, hash.RecommendedHashType, data, true)
	if err != nil {
		t.Fatal(err)
	}

	// Abort on the error.
	if _, err := exchangeMessage(remote, false, &SOSyncMessage{Body: &SOSyncMessage_Proof{Proof: proof}}); err != nil {
		t.Fatal(err)
	}
	if _, err := exchangeMessage(remote, false, &SOSyncMessage{}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil || errors.Is(err, ErrAccessDenied) {
			t.Fatalf("missing admission result = %v, want protocol error", err)
		}
	case <-ctx.Done():
		t.Fatal("invalid admission did not close the stream")
	}
	select {
	case <-admissions:
		t.Fatal("missing admission was reported as a peer access decision")
	default:
	}
}

// authenticationStream records each complete outbound packet before transport admission.
type authenticationStream struct {
	// Conn supplies the real stream and its deadline and cancellation behavior.
	net.Conn
	// messages records the bounded test exchange, including a send blocked in transport.
	messages chan *SOSyncMessage
}

// Write records a packet without changing the underlying stream's blocking behavior.
func (s *authenticationStream) Write(data []byte) (int, error) {
	message := &SOSyncMessage{}
	if len(data) >= 4 && message.UnmarshalVT(data[4:]) == nil {
		s.messages <- message
	}
	return s.Conn.Write(data)
}

// newAuthenticationPeer creates a host with a locally held shared checkpoint.
func newAuthenticationPeer(t *testing.T, soID string, key crypto.PrivKey, state *sobject.SOState) *SOSync {

	// helper.
	t.Helper()
	id, err := peer.IDFromPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	host, _ := newMemHost(soID, state.CloneVT())
	t.Cleanup(host.ClearContext)
	return NewSOSync(gateLogger(), nil, soID, id, key, host, nil)
}

// authenticationState establishes the trusted participants before the stream exists.
func authenticationState(t *testing.T, soID string, owner, reader crypto.PrivKey) *sobject.SOState {

	// helper.
	t.Helper()
	state := &sobject.SOState{
		Config: &sobject.SharedObjectConfig{Participants: []*sobject.SOParticipantConfig{
			participantCfg(mustPeerIDStr(t, owner), sobject.SOParticipantRole_SOParticipantRole_OWNER),
			participantCfg(mustPeerIDStr(t, reader), sobject.SOParticipantRole_SOParticipantRole_READER),
		}},
	}
	trustSnapshotConfig(t, soID, state, owner)
	return state
}

// TestParticipantAuthenticationRejectsReboundProof tests every signed binding field.
func TestParticipantAuthenticationRejectsReboundProof(t *testing.T) {

	// mustKeyPair key.
	key := mustKeyPair(t)
	transcript := &SOSyncAuthTranscript{
		SharedObjectId: "authentication-object", SenderTransport: []byte("sender"), ReceiverTransport: []byte("receiver"),
		SenderNonce: []byte("sender-challenge"), ReceiverNonce: []byte("receiver-challenge"),
	}
	data, err := transcript.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	proof, err := peer.NewSignature(authenticationContext, key, hash.RecommendedHashType, data, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifyParticipantProof(transcript, proof); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*SOSyncAuthTranscript){
		func(v *SOSyncAuthTranscript) { v.SharedObjectId = "another-object" },
		func(v *SOSyncAuthTranscript) { v.SenderTransport = []byte("another-sender") },
		func(v *SOSyncAuthTranscript) { v.ReceiverTransport = []byte("another-receiver") },
		func(v *SOSyncAuthTranscript) { v.SenderNonce = []byte("another-sender-challenge") },
		func(v *SOSyncAuthTranscript) { v.ReceiverNonce = []byte("another-receiver-challenge") },
		func(v *SOSyncAuthTranscript) {
			v.SenderTransport, v.ReceiverTransport = v.ReceiverTransport, v.SenderTransport
			v.SenderNonce, v.ReceiverNonce = v.ReceiverNonce, v.SenderNonce
		},
	} {
		changed := transcript.CloneVT()
		mutate(changed)
		if _, err := verifyParticipantProof(changed, proof); err == nil {
			t.Fatal("accepted a proof rebound to another stream, object, or direction")
		}
	}
}

// TestParticipantAuthenticationDeniesBeforeDisclosure runs both real stream owners.
func TestParticipantAuthenticationDeniesBeforeDisclosure(t *testing.T) {
	for _, test := range []struct {
		// name identifies the admission state being exercised.
		name string
		// outsider signs with an identity absent from the held configuration.
		outsider bool
		// removed leaves the remote device on its old configuration after local removal.
		removed bool
	}{
		{name: "authorized"},
		{name: "outsider", outsider: true},
		{name: "removed device reconnect", removed: true},
	} {
		t.Run(test.name, func(t *testing.T) {

			// withTimeout ctx,cancel via context.
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			const soID = "authentication-stream"
			owner, reader := mustKeyPair(t), mustKeyPair(t)
			state := authenticationState(t, soID, owner, reader)
			remoteKey := reader

			// Check the condition before continuing.
			if test.outsider {
				remoteKey = mustKeyPair(t)
			}
			local := newAuthenticationPeer(t, soID, owner, state)
			remote := newAuthenticationPeer(t, soID, remoteKey, state)
			localHealth := sobject.NewSharedObjectReadyHealth(sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT)
			remoteHealth := localHealth.CloneVT()
			local.peerAdmission = func(remoteID peer.ID, accepted bool) {
				localHealth = localHealth.WithSyncPeerAdmission(remoteID.String(), accepted)
			}

			// func.
			remote.peerAdmission = func(remoteID peer.ID, accepted bool) {
				remoteHealth = remoteHealth.WithSyncPeerAdmission(remoteID.String(), accepted)
			}
			if test.removed {
				removed, err := sobject.RemoveSOParticipant(ctx, local.soHost, remote.localObjectPeerID.String(), owner, nil)
				if err != nil || !removed {
					t.Fatalf("remove before reconnect = %v, %v", removed, err)
				}
			}
			denied := test.outsider || test.removed
			left, right := net.Pipe()
			t.Cleanup(func() { left.Close(); right.Close() })
			observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 32)}

			// make done.
			done := make(chan error, 2)
			go func() { done <- local.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b") }()
			go func() { done <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
			if !denied {
				waitAuthenticationData(t, ctx, observed.messages)
				cancel()
			}
			for range 2 {
				select {
				case err := <-done:
					if denied && !errors.Is(err, ErrAccessDenied) {
						t.Fatalf("denied participant result = %v", err)
					}
				case <-time.After(time.Second):
					t.Fatal("stream owner failed to join")
				}
			}
			if denied {
				for len(observed.messages) > 0 {
					message := <-observed.messages
					if message.GetSnapshot() != nil || message.GetHead() != nil || message.GetHistoryPage() != nil {
						t.Fatal("unauthorized stream received object data")
					}
				}
			}

			// Only the removed device accepted its peer locally and received an explicit denial.
			if len(localHealth.GetSyncDeniedPeerIds()) != 0 {
				t.Fatal("local rejection was reported as a remote denial")
			}
			if test.removed {
				if got := remoteHealth.GetSyncDeniedPeerIds(); len(got) != 1 || got[0] != local.localObjectPeerID.String() {
					t.Fatalf("removed device's denied source = %v", got)
				}
			} else if len(remoteHealth.GetSyncDeniedPeerIds()) != 0 {
				t.Fatal("untrusted or accepted peer was reported as a denied source")
			}
			if remoteHealth.GetStatus() != sobject.SharedObjectHealthStatus_SHARED_OBJECT_HEALTH_STATUS_READY {
				t.Fatal("remote denial changed local mount readiness")
			}
		})
	}
}

// TestParticipantRevocationNotifiesConnectedPeer preserves the old replica while
// delivering an explicit denial over the already authenticated stream.
func TestParticipantRevocationNotifiesConnectedPeer(t *testing.T) {

	// withTimeout ctx,cancel via context.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	const soID = "authentication-live-revocation"
	owner, reader := mustKeyPair(t), mustKeyPair(t)

	// authenticationState state.
	state := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, state)
	remote := newAuthenticationPeer(t, soID, reader, state)
	denied := make(chan peer.ID, 1)
	remote.peerAdmission = func(remoteID peer.ID, accepted bool) {
		if !accepted {
			denied <- remoteID
		}
	}

	// Start the real paired exchange and remove access once object traffic has begun.
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 32)}
	done := make(chan error, 2)
	go func() { done <- local.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b") }()
	go func() { done <- remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()

	// waitAuthenticationData.
	waitAuthenticationData(t, ctx, observed.messages)
	removed, err := sobject.RemoveSOParticipant(ctx, local.soHost, remote.localObjectPeerID.String(), owner, nil)
	if err != nil || !removed {
		t.Fatalf("remove = %v, %v", removed, err)
	}
	select {
	case source := <-denied:
		if source != local.localObjectPeerID {
			t.Fatalf("denied source = %v", source)
		}
	case <-ctx.Done():
		t.Fatal("connected peer did not receive the revocation notice")
	}
	for range 2 {
		select {
		case <-done:
		case <-ctx.Done():
			t.Fatal("revoked stream failed to close")
		}
	}

	// The notice reveals neither the removed configuration nor any operation history.
	for len(observed.messages) != 0 {
		message := <-observed.messages
		if snapshot := message.GetSnapshot(); snapshot != nil {
			decoded := &sobject.SOState{}
			if err := decoded.UnmarshalVT(snapshot.GetSoState()); err != nil {
				t.Fatal(err)
			}
			if !decoded.GetConfig().EqualVT(state.GetConfig()) || !decoded.GetCheckpoint().EqualVT(state.GetCheckpoint()) {
				t.Fatal("revocation disclosed new configuration or history")
			}
		}
	}
	retained, err := remote.soHost.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !retained.GetConfig().EqualVT(state.GetConfig()) {
		t.Fatal("revocation rewrote the removed peer's retained configuration")
	}
}

// TestParticipantRevocationClosesBlockedSnapshot proves the authority watch can
// stop a stream whose authorized initial send is blocked in the transport.
func TestParticipantRevocationClosesBlockedSnapshot(t *testing.T) {

	// withTimeout ctx,cancel via context.
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	const soID = "authentication-revocation"
	owner, reader := mustKeyPair(t), mustKeyPair(t)

	// authenticationState state.
	state := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, state)
	remote := newAuthenticationPeer(t, soID, reader, state)
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })

	// make observed.
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 32)}
	done := make(chan error, 1)
	go func() { done <- local.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b") }()
	if _, err := remote.authenticate(ctx, stream_packet.NewSession(right, 64*1024), "transport-b", "transport-a"); err != nil {
		t.Fatal(err)
	}
	waitAuthenticationData(t, ctx, observed.messages)
	removed, err := sobject.RemoveSOParticipant(ctx, local.soHost, remote.localObjectPeerID.String(), owner, nil)
	if err != nil || !removed {
		t.Fatalf("remove = %v, %v", removed, err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, ErrAccessDenied) {
			t.Fatalf("revoked stream result = %v, want access denied", err)
		}
	case <-time.After(time.Second):
		t.Fatal("revocation did not close and join the blocked stream")
	}
}

// waitAuthenticationData waits for admission of the first authenticated data frame.
func waitAuthenticationData(t *testing.T, ctx context.Context, messages <-chan *SOSyncMessage) {
	t.Helper()
	for {
		select {
		case message := <-messages:
			if message.GetHead() != nil {
				return
			}
		case <-ctx.Done():
			t.Fatal("authenticated head did not reach transport admission")
		}
	}
}

// TestParticipantAuthenticationRejectsPrematureData verifies no snapshot is
// disclosed when the first remote message is data or a reflected challenge.
func TestParticipantAuthenticationRejectsPrematureData(t *testing.T) {
	for _, reflectChallenge := range []bool{false, true} {
		t.Run(map[bool]string{false: "premature snapshot", true: "reflected challenge"}[reflectChallenge], func(t *testing.T) {

			// withTimeout ctx,cancel via context.
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			const soID = "authentication-premature"
			owner, reader := mustKeyPair(t), mustKeyPair(t)
			local := newAuthenticationPeer(t, soID, owner, authenticationState(t, soID, owner, reader))

			// pipe left,right via net.
			left, right := net.Pipe()
			t.Cleanup(func() { left.Close(); right.Close() })
			observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 32)}
			done := make(chan error, 1)
			go func() { done <- local.runStream(ctx, gateLogger(), observed, "transport-a", "transport-b") }()

			// newSession remote via stream_packet.
			remote := stream_packet.NewSession(right, 64*1024)
			challenge := &SOSyncMessage{}
			if err := remote.RecvMsg(challenge); err != nil {
				t.Fatal(err)
			}
			reply := &SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{SoState: []byte("premature")}}}
			if reflectChallenge {
				reply = challenge
			}
			if err := remote.SendMsg(reply); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("accepted invalid authentication input")
				}
			case <-time.After(time.Second):
				t.Fatal("invalid authentication did not close the stream")
			}
			for len(observed.messages) > 0 {
				if message := <-observed.messages; message.GetChallenge() == nil {
					t.Fatal("disclosed more than a challenge before authentication")
				}
			}
		})
	}
}
