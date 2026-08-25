package webrtc

// RED reproduction for the Mercury v5 hosted-join stale-answer drops.
//
// Drives the real production boundary end to end: handleSignalPeerResolver
// delivers through deliverSignal into the ingress lease, the keyed ref-count
// runs the real sessionTracker.execute with real Pion peer connections, and
// the remote answerer is a real Pion answer-side PeerConnection. No fence or
// guard behavior is modified.
//
// Sequence under test mirrors the Mercury v5 hosted join: the WebRTC offerer
// regenerates its tracker generation while its answer is still in flight, and
// the era-A answers arrive at the successor generation. The target invariant
// is that a hosted join loses no answers across tracker regeneration. The
// observed behavior today is two "dropping stale answer: offer id does not
// match the pending local offer" drops, after which negotiation stalls until
// ICE fails - exactly the Mercury v5 signature.

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	inmem "github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/keyed"
	pion_webrtc "github.com/pion/webrtc/v4"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	signaling "github.com/s4wave/spacewave/net/signaling"
)

// staleAnswerDropPrefix is the consumer fence log line the defect produces.
const staleAnswerDropPrefix = "dropping stale answer"

// redXmitSession stands in for the outbound signaling session execute opens
// through ExSignalPeer. Every transmitted signal lands on sendCh.
type redXmitSession struct {
	localPeerID  peer.ID
	remotePeerID peer.ID
	sendCh       chan []byte
}

func (s *redXmitSession) GetLocalPeerID() peer.ID  { return s.localPeerID }
func (s *redXmitSession) GetRemotePeerID() peer.ID { return s.remotePeerID }
func (s *redXmitSession) Recv(ctx context.Context) ([]byte, error) {
	<-ctx.Done()
	return nil, context.Canceled
}

func (s *redXmitSession) Send(ctx context.Context, msg []byte) error {
	// Copy before parking: the caller scrubs its buffer once Send returns.
	cp := make([]byte, len(msg))
	copy(cp, msg)
	select {
	case s.sendCh <- cp:
		return nil
	case <-ctx.Done():
		return context.Canceled
	}
}

// redSignalController resolves SignalPeer directives with one shared
// scripted session, standing in for the signaling relay.
type redSignalController struct {
	sess signaling.SignalPeerSession
}

func (c *redSignalController) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if _, ok := di.GetDirective().(signaling.SignalPeer); ok {
		resolvers, _ := directive.R(&redSignalResolver{sess: c.sess}, nil)
		return resolvers, nil
	}
	return nil, nil
}
func (c *redSignalController) Execute(ctx context.Context) error { return nil }
func (c *redSignalController) Close() error                      { return nil }
func (c *redSignalController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID+"/signal-red-test", Version, "test signal peer provider")
}

type redSignalResolver struct {
	sess signaling.SignalPeerSession
}

func (r *redSignalResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	handler.ClearValues()
	if vid, accepted := handler.AddValue(r.sess); accepted {
		handler.AddValueRemovedCallback(vid, func() {})
	}
	<-ctx.Done()
	return ctx.Err()
}

// redIdentity generates a local and remote identity with both private keys,
// forcing the local peer to sort before the remote so the local side takes
// the deterministic offerer role.
type redIdentity struct {
	ident        *hostedFlowIdentity
	localPriv    crypto.PrivKey
	localPub     crypto.PubKey
	localPeerID  peer.ID
	remotePriv   crypto.PrivKey
	remotePeerID peer.ID
}

func newRedIdentity(t *testing.T) *redIdentity {
	t.Helper()
	for {
		localPriv, localPub, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err.Error())
		}
		localPeerID, err := peer.IDFromPrivateKey(localPriv)
		if err != nil {
			t.Fatal(err.Error())
		}
		remotePriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			t.Fatal(err.Error())
		}
		remotePeerID, err := peer.IDFromPrivateKey(remotePriv)
		if err != nil {
			t.Fatal(err.Error())
		}
		if strings.Compare(localPeerID.String(), remotePeerID.String()) < 0 {
			return &redIdentity{
				ident: &hostedFlowIdentity{
					localPriv:    localPriv,
					localPub:     localPub,
					localPeerID:  localPeerID,
					remotePeerID: remotePeerID,
				},
				localPriv:    localPriv,
				localPub:     localPub,
				localPeerID:  localPeerID,
				remotePriv:   remotePriv,
				remotePeerID: remotePeerID,
			}
		}
	}
}

// awaitRedOutboundSignal decodes the next outbound signal of the given body
// kind, skipping anything else (ICE trickle, end-of-candidates).
func awaitRedOutboundSignal(
	t *testing.T,
	ch <-chan []byte,
	remotePriv crypto.PrivKey,
	wantSdpOffer bool,
) *WebRtcSignal {
	t.Helper()
	deadline := time.After(hostedFlowTimeout)
	for {
		select {
		case msg := <-ch:
			sig, err := DecodeWebRtcSignal(msg, remotePriv)
			if err != nil {
				t.Fatalf("decode outbound signal (%d bytes): %v", len(msg), err)
			}
			switch b := sig.GetBody().(type) {
			case *WebRtcSignal_Sdp:
				if wantSdpOffer && b.Sdp.GetSdpType() == "offer" {
					return sig
				}
			case *WebRtcSignal_Ice:
				// gathering noise; skip
			case *WebRtcSignal_RequestOffer:
				// not expected outbound; skip
			}
		case <-deadline:
			t.Fatal("timed out waiting for the expected outbound signal")
			return nil
		}
	}
}

// startRedResolver drives one inbound signaling session through the
// production resolver loop with its own lifetime.
func startRedResolver(
	ctx context.Context,
	tpt *WebRTC,
	signalSession *testSignalPeerSession,
) (*handleSignalPeerResolver, context.CancelFunc) {
	resolverCtx, cancel := context.WithCancel(tpt.ctx)
	resolver := &handleSignalPeerResolver{t: tpt, sess: signalSession}
	go func() {
		_ = resolver.Resolve(resolverCtx, nil)
	}()
	return resolver, cancel
}

// buildRedTaggedOffer builds one real, valid offer SDP signal tagged with its
// own generation digest, encrypted for the local transport.
func buildRedTaggedOffer(t *testing.T, api *pion_webrtc.API, pub crypto.PubKey) []byte {
	t.Helper()
	offerPC, err := api.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer offerPC.Close()
	if _, err := offerPC.CreateDataChannel(dataChannelID, nil); err != nil {
		t.Fatal(err.Error())
	}
	offer, err := offerPC.CreateOffer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	msg, err := EncodeWebRtcSignal(&WebRtcSignal{
		Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{
			TxSeqno: 1,
			SdpType: offer.Type.String(),
			Sdp:     offer.SDP,
			OfferId: offerDigest(offer.SDP),
		}},
	}, pub)
	if err != nil {
		t.Fatal(err.Error())
	}
	return msg
}

// TestHostedJoinKeepsAnswersAcrossTrackerRegeneration pins the hosted-join
// invariant broken by Mercury v5: when the offerer's tracker generation is
// retired while its answer is still in flight, the successor generation adopts
// the outstanding-offer session, retransmits the identical offer, and the
// era-A answer - plus a redelivered copy of it - correlates instead of being
// dropped as stale. Today's pre-fix behavior drops both answers and the join
// stalls until ICE fails.
func TestHostedJoinKeepsAnswersAcrossTrackerRegeneration(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	ident := newRedIdentity(t)
	logs := newHostedFlowLogs()
	tpt := newHostedFlowTransport(ctx, ident.ident, logs)

	// Wire the outbound signaling session through a real bus so
	// sessionTracker.execute resolves ExSignalPeer exactly as in production.
	xmit := &redXmitSession{
		localPeerID:  ident.localPeerID,
		remotePeerID: ident.remotePeerID,
		sendCh:       make(chan []byte, 64),
	}
	b := inmem.NewBus(directive_controller.NewController(ctx, logs.entry))
	if _, err := b.AddController(ctx, &redSignalController{sess: xmit}, nil); err != nil {
		t.Fatal(err.Error())
	}
	tpt.b = b

	// Production tracker factory: every generation runs the real execute loop.
	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		tpt.newSessionTracker,
		keyed.WithBackoff[string, *sessionTracker](func(string) cbackoff.BackOff {
			return new(cbackoff.ZeroBackOff)
		}),
	)
	tpt.sessionTrackers.SetContext(tpt.ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)

	// Generation A: establish the lease, let the real execute loop mint and
	// transmit its offer, then build the matching answer and hold it in
	// flight, exactly where the regeneration strikes.
	signalSessionA := newHostedFlowSignalSession(ident.ident, 8)
	_, cancelA := startRedResolver(ctx, tpt, signalSessionA)
	defer cancelA()

	signalSessionA.recvCh <- newHostedFlowMarker(t, ident.localPub)
	offerASig := awaitRedOutboundSignal(t, xmit.sendCh, ident.remotePriv, true)
	offerA := offerASig.GetBody().(*WebRtcSignal_Sdp).Sdp
	hA := sha256.Sum256([]byte(offerA.GetSdp()))

	answerPC, err := tpt.webrtcApi.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer func() { _ = answerPC.Close() }()
	if err := answerPC.SetRemoteDescription(pion_webrtc.SessionDescription{
		Type: pion_webrtc.SDPTypeOffer,
		SDP:  offerA.GetSdp(),
	}); err != nil {
		t.Fatal(err.Error())
	}
	answerA, err := answerPC.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	eraAAnswer, err := EncodeWebRtcSignal(&WebRtcSignal{
		Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{
			TxSeqno: 0,
			SdpType: answerA.Type.String(),
			Sdp:     answerA.SDP,
			OfferId: hA[:],
		}},
	}, ident.localPub)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Retire generation A while the era-A answer is still in flight. An SDP
	// offer arriving at an offerer is a fatal role violation, which returns
	// from execute without recording a session fatal error - the production
	// churn shape this fix targets. The signaling session stays alive, so the
	// ingress lease stays discoverable between generations.
	signalSessionA.recvCh <- buildRedTaggedOffer(t, tpt.webrtcApi, ident.localPub)
	if !logs.awaitPrefix(t, "session tracker exited") {
		t.Fatal("generation A did not retire")
	}

	// The next delivery rebinds a successor onto the same lease; the successor
	// adopts the handed-over session and retransmits the identical outstanding
	// offer instead of minting a new generation.
	signalSessionA.recvCh <- newHostedFlowMarker(t, ident.localPub)
	retransSig := awaitRedOutboundSignal(t, xmit.sendCh, ident.remotePriv, true)
	retrans := retransSig.GetBody().(*WebRtcSignal_Sdp).Sdp
	if retrans.GetSdp() != offerA.GetSdp() || !bytes.Equal(retrans.GetOfferId(), hA[:]) {
		t.Fatal("successor did not retransmit the identical outstanding offer generation")
	}

	// Deliver the held era-A answer, then a redelivered copy the way the hub
	// replays era material to the peer's second signaling session.
	pushRedSignal := func() {
		// Deliver a fresh copy each time: the resolver scrubs the buffer
		// after decoding, exactly as production hands off decoded signals.
		cp := make([]byte, len(eraAAnswer))
		copy(cp, eraAAnswer)
		signalSessionA.recvCh <- cp
	}
	pushRedSignal()
	pushRedSignal()

	// The invariant: zero stale-answer drops across the whole sequence.
	drainDeadline := time.After(3 * time.Second)
	var drops []string
drain:
	for {
		select {
		case msg := <-logs.ch:
			if strings.HasPrefix(msg, staleAnswerDropPrefix) {
				drops = append(drops, msg)
			}
		case <-drainDeadline:
			break drain
		}
	}
	if len(drops) > 0 {
		t.Fatalf("hosted join dropped %d answers across tracker regeneration:\n%s",
			len(drops), strings.Join(drops, "\n"))
	}
}
