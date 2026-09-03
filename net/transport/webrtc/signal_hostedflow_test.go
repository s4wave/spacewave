package webrtc

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/keyed"
	pion_webrtc "github.com/pion/webrtc/v4"
	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// hostedFlowTimeout bounds every blocking wait in the hosted-flow tests.
const hostedFlowTimeout = 30 * time.Second

// hostedFlowIdentity carries the peer identities driving one hosted-flow test.
type hostedFlowIdentity struct {
	localPriv    crypto.PrivKey
	localPub     crypto.PubKey
	localPeerID  peer.ID
	remotePeerID peer.ID
}

// newHostedFlowIdentity generates a fresh local and remote peer identity.
func newHostedFlowIdentity(t *testing.T) *hostedFlowIdentity {
	t.Helper()
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
	return &hostedFlowIdentity{
		localPriv:    localPriv,
		localPub:     localPub,
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
	}
}

// hostedFlowLogs collects transport log messages so a test can observe which
// fence dropped a signal without coupling to internal state.
type hostedFlowLogs struct {
	entry *logrus.Entry
	ch    chan string
}

// newHostedFlowLogs builds a debug-level logger that forwards every message
// into an ordered channel.
func newHostedFlowLogs() *hostedFlowLogs {
	ch := make(chan string, 128)
	lg := logrus.New()
	lg.SetLevel(logrus.DebugLevel)
	lg.AddHook(&hostedFlowLogHook{ch: ch})
	return &hostedFlowLogs{entry: logrus.NewEntry(lg), ch: ch}
}

// awaitPrefix reports whether one message with the prefix arrives in time.
func (l *hostedFlowLogs) awaitPrefix(t *testing.T, prefix string) bool {
	t.Helper()
	deadline := time.After(hostedFlowTimeout)
	for {
		select {
		case msg := <-l.ch:
			if strings.HasPrefix(msg, prefix) {
				return true
			}
		case <-deadline:
			return false
		}
	}
}

// sawPrefix reports whether one already-buffered message carries the prefix.
func (l *hostedFlowLogs) sawPrefix(prefix string) bool {
	for {
		select {
		case msg := <-l.ch:
			if strings.HasPrefix(msg, prefix) {
				return true
			}
		default:
			return false
		}
	}
}

// hostedFlowLogHook forwards log messages into the test channel.
type hostedFlowLogHook struct {
	ch chan string
}

// Fire implements logrus.Hook.
func (h *hostedFlowLogHook) Fire(entry *logrus.Entry) error {
	select {
	case h.ch <- entry.Message:
	default:
	}
	return nil
}

// Levels implements logrus.Hook.
func (h *hostedFlowLogHook) Levels() []logrus.Level {
	return []logrus.Level{
		logrus.DebugLevel,
		logrus.InfoLevel,
		logrus.WarnLevel,
		logrus.ErrorLevel,
	}
}

// newHostedFlowTransport constructs a WebRTC transport with a live Pion API
// and no controllers, ready for a test-installed session tracker.
func newHostedFlowTransport(ctx context.Context, ident *hostedFlowIdentity, logs *hostedFlowLogs) *WebRTC {
	settingEngine := pion_webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	return &WebRTC{
		ctx:              ctx,
		le:               logs.entry,
		conf:             &Config{},
		peerID:           ident.localPeerID,
		privKey:          ident.localPriv,
		incomingSessions: make(map[string]*signalIngress),
		webrtcApi:        pion_webrtc.NewAPI(pion_webrtc.WithSettingEngine(settingEngine)),
		webrtcConf:       &pion_webrtc.Configuration{},
	}
}

// hostedFlowGeneration exposes one live fake session generation to the test.
type hostedFlowGeneration struct {
	index     int32
	tracker   *sessionTracker
	execution *sessionTrackerExecution
	sess      *session
	offerSDP  string
	offerID   []byte
}

// hostedFlowDelivery reports one signal a fake generation pulled from its
// execution, together with the post-ingest Pion state.
type hostedFlowDelivery struct {
	gen         int32
	body        string
	err         error
	remoteDesc  bool
	bufferedIce int
}

// runHostedFlowGeneration executes one fake session generation. It publishes a
// real Pion session with a local offer exactly as transmitLocalNegotiation tags
// the active generation identity, then ingests every delivered signal through
// the production ingestRemoteSignal path and reports the post-ingest state.
func runHostedFlowGeneration(
	ctx context.Context,
	tkr *sessionTracker,
	index int32,
	gens chan<- *hostedFlowGeneration,
	deliveries chan<- hostedFlowDelivery,
	hold <-chan struct{},
	regen <-chan struct{},
) error {
	execution := tkr.beginExecution()
	defer tkr.retireExecution(execution)
	sess, _, err := tkr.newSession()
	if err != nil {
		return pkgerrors.Wrap(err, "construct hosted-flow session")
	}
	defer sess.close()

	localDesc, err := sess.pc.CreateOffer(nil)
	if err != nil {
		return pkgerrors.Wrap(err, "create hosted-flow offer")
	}
	if err := sess.pc.SetLocalDescription(localDesc); err != nil {
		return pkgerrors.Wrap(err, "set hosted-flow local offer")
	}
	offerSum := sha256.Sum256([]byte(localDesc.SDP))
	sess.pendingOfferID = offerSum[:]

	gens <- &hostedFlowGeneration{
		index:     index,
		tracker:   tkr,
		execution: execution,
		sess:      sess,
		offerSDP:  localDesc.SDP,
		offerID:   offerSum[:],
	}

	// Park off the signal stream until the test releases this generation, so a
	// delivery can be held against this execution while it retires.
	select {
	case <-ctx.Done():
		return context.Canceled
	case <-regen:
		return context.Canceled
	case <-hold:
	}

	lastAppliedRemoteSdp := ""
	pendingRemoteIce := make([]pendingRemoteCandidate, 0)
	remoteICE := remoteICECandidateApplier{add: sess.pc.AddICECandidate}

	for {
		var incoming *incomingSignal
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-regen:
			return context.Canceled
		case incoming = <-execution.rxSignal:
		}
		sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			sess.acceptIncomingSignalLocked(incoming)
		})

		var rxSdp *WebRtcSdp
		var rxIce *WebRtcIce
		switch b := incoming.sig.GetBody().(type) {
		case *WebRtcSignal_Sdp:
			rxSdp = b.Sdp
		case *WebRtcSignal_Ice:
			rxIce = b.Ice
		}
		phase := "hosted-flow test"
		ingestErr := tkr.ingestRemoteSignal(
			sess,
			rxSdp,
			rxIce,
			0,
			&lastAppliedRemoteSdp,
			&remoteICE,
			&pendingRemoteIce,
			func(*WebRtcSignal) {},
			tkr.le,
			&phase,
		)
		deliveries <- hostedFlowDelivery{
			gen:         index,
			body:        classifySignal(incoming.sig),
			err:         ingestErr,
			remoteDesc:  sess.pc.RemoteDescription() != nil,
			bufferedIce: len(pendingRemoteIce),
		}
	}
}

// startHostedFlowTracker installs a keyed tracker factory that runs one fake
// generation per construction. hold releases each generation's signal stream;
// regen retires the live generation. The zero backoff restarts immediately so
// the next delivery constructs or rebinds the successor generation.
func startHostedFlowTracker(
	t *testing.T,
	tpt *WebRTC,
	remotePeerID peer.ID,
	hold <-chan struct{},
	regen <-chan struct{},
) (<-chan *hostedFlowGeneration, <-chan hostedFlowDelivery) {
	t.Helper()
	gens := make(chan *hostedFlowGeneration, 8)
	deliveries := make(chan hostedFlowDelivery, 64)
	var generations atomic.Int32

	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			tkr := &sessionTracker{
				w:       tpt,
				le:      tpt.le,
				key:     key,
				peerID:  remotePeerID,
				offerer: true,
			}
			index := generations.Add(1) - 1
			return func(ctx context.Context) error {
				return runHostedFlowGeneration(ctx, tkr, index, gens, deliveries, hold, regen)
			}, tkr
		},
		keyed.WithBackoff[string, *sessionTracker](func(string) cbackoff.BackOff {
			return new(cbackoff.ZeroBackOff)
		}),
	)
	tpt.sessionTrackers.SetContext(tpt.ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)
	return gens, deliveries
}

// startHostedFlowResolver drives one signaling session through the production
// resolver loop and returns its exit channel and resolver pointer.
func startHostedFlowResolver(
	ctx context.Context,
	tpt *WebRTC,
	signalSession *testSignalPeerSession,
) (<-chan error, *handleSignalPeerResolver) {
	resolverErr := make(chan error, 1)
	resolver := &handleSignalPeerResolver{t: tpt, sess: signalSession}
	go func() {
		resolverErr <- resolver.Resolve(ctx, nil)
	}()
	return resolverErr, resolver
}

// newHostedFlowSignalSession builds a signaling session whose Recv stream the
// test fills directly.
func newHostedFlowSignalSession(ident *hostedFlowIdentity, buf int) *testSignalPeerSession {
	return &testSignalPeerSession{
		localPeerID:  ident.localPeerID,
		remotePeerID: ident.remotePeerID,
		recvCh:       make(chan []byte, buf),
		recvStarted:  make(chan struct{}, buf+4),
	}
}

// newHostedFlowAnswer builds an answer SDP signal for offerSDP tagged with
// offerID, using a real Pion answerer for byte-compatible descriptions.
func newHostedFlowAnswer(t *testing.T, api *pion_webrtc.API, pub crypto.PubKey, offerSDP string, offerID []byte) []byte {
	t.Helper()
	answerPC, err := api.NewPeerConnection(pion_webrtc.Configuration{})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer answerPC.Close()
	if err := answerPC.SetRemoteDescription(pion_webrtc.SessionDescription{
		Type: pion_webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}); err != nil {
		t.Fatal(err.Error())
	}
	answer, err := answerPC.CreateAnswer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	msg, err := EncodeWebRtcSignal(&WebRtcSignal{
		Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{
			TxSeqno: 1,
			SdpType: answer.Type.String(),
			Sdp:     answer.SDP,
			OfferId: offerID,
		}},
	}, pub)
	if err != nil {
		t.Fatal(err.Error())
	}
	return msg
}

// newHostedFlowTrickle builds an ICE candidate signal tagged with offerID; a
// nil offerID produces untagged era material.
func newHostedFlowTrickle(t *testing.T, pub crypto.PubKey, offerID []byte) []byte {
	t.Helper()
	mlineIndex := uint16(0)
	ice, err := NewWebRtcIce(&pion_webrtc.ICECandidateInit{
		Candidate:     "candidate:1 1 udp 2130706431 10.5.0.2 54504 typ host",
		SDPMLineIndex: &mlineIndex,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	ice.OfferId = offerID
	msg, err := EncodeWebRtcSignal(&WebRtcSignal{Body: &WebRtcSignal_Ice{Ice: ice}}, pub)
	if err != nil {
		t.Fatal(err.Error())
	}
	return msg
}

// newHostedFlowMarker builds an exempt request_offer renegotiation wake.
func newHostedFlowMarker(t *testing.T, pub crypto.PubKey) []byte {
	t.Helper()
	msg, err := EncodeWebRtcSignal(
		&WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: 7}},
		pub,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	return msg
}

// newHostedFlowUntaggedOffer builds an SDP description carrying no offer id.
func newHostedFlowUntaggedOffer(t *testing.T, api *pion_webrtc.API, pub crypto.PubKey) []byte {
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
		}},
	}, pub)
	if err != nil {
		t.Fatal(err.Error())
	}
	return msg
}

// awaitHostedFlowGeneration receives one published generation.
func awaitHostedFlowGeneration(t *testing.T, gens <-chan *hostedFlowGeneration) *hostedFlowGeneration {
	t.Helper()
	select {
	case gen := <-gens:
		return gen
	case <-time.After(hostedFlowTimeout):
		t.Fatal("timed out waiting for a hosted-flow generation")
		return nil
	}
}

// awaitHostedFlowDelivery receives one reported signal delivery.
func awaitHostedFlowDelivery(t *testing.T, deliveries <-chan hostedFlowDelivery) hostedFlowDelivery {
	t.Helper()
	select {
	case d := <-deliveries:
		return d
	case <-time.After(hostedFlowTimeout):
		t.Fatal("timed out waiting for a hosted-flow delivery")
		return hostedFlowDelivery{}
	}
}

// assertHostedFlowSuccessor requires the successor generation to run on a
// fresh Pion session and execution, whatever tracker object keyed reused.
func assertHostedFlowSuccessor(t *testing.T, genA, genB *hostedFlowGeneration) {
	t.Helper()
	if genB.sess == genA.sess {
		t.Fatal("successor generation reused the retired session")
	}
	if genB.execution == genA.execution {
		t.Fatal("successor generation reused the retired execution")
	}
}

// waitForDetachedSignalIngress waits until the peer's ingress lease is
// discoverable with no live tracker attached and the resolver still holds
// membership.
func waitForDetachedSignalIngress(
	t *testing.T,
	tpt *WebRTC,
	peerIDStr string,
	resolver *handleSignalPeerResolver,
) *signalIngress {
	t.Helper()
	for {
		var detached *signalIngress
		var waitCh <-chan struct{}
		tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if ingress := tpt.incomingSessions[peerIDStr]; ingress != nil && ingress.tracker == nil {
				if _, ok := ingress.resolvers[resolver]; ok {
					detached = ingress
				}
			}
			waitCh = getWaitCh()
		})
		if detached != nil {
			return detached
		}
		select {
		case <-waitCh:
		case <-time.After(hostedFlowTimeout):
			t.Fatal("ingress was not discoverable while detached between generations")
			return nil
		}
	}
}

// TestHostedFlowDoubleSessionStaleAnswerOrdering pins the hosted double-session
// defect shape at handler level: after the peer's signal tracker regenerates,
// an answer from the retired session's generation must not touch the successor
// session's Pion state, and matching-generation material must still deliver.
func TestHostedFlowDoubleSessionStaleAnswerOrdering(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	ident := newHostedFlowIdentity(t)
	logs := newHostedFlowLogs()
	tpt := newHostedFlowTransport(ctx, ident, logs)
	hold := make(chan struct{})
	regen := make(chan struct{})
	gens, deliveries := startHostedFlowTracker(t, tpt, ident.remotePeerID, hold, regen)

	signalSession := newHostedFlowSignalSession(ident, 4)
	resolverErr, _ := startHostedFlowResolver(ctx, tpt, signalSession)

	// Establish session A: the marker acquires the lease and the tracker, and
	// the matching answer completes the negotiation.
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	genA := awaitHostedFlowGeneration(t, gens)
	if genA.tracker == nil {
		t.Fatal("establishment marker acquired no tracker")
	}
	hold <- struct{}{}
	established := awaitHostedFlowDelivery(t, deliveries)
	if established.gen != genA.index || established.body != "request_offer" || established.err != nil {
		t.Fatalf("lease establishment marker failed: %+v", established)
	}
	signalSession.recvCh <- newHostedFlowAnswer(t, tpt.webrtcApi, ident.localPub, genA.offerSDP, genA.offerID)
	deliveredA := awaitHostedFlowDelivery(t, deliveries)
	if deliveredA.gen != genA.index || deliveredA.body != "sdp/answer" || deliveredA.err != nil || !deliveredA.remoteDesc {
		t.Fatalf("session A did not establish cleanly: %+v", deliveredA)
	}

	// Regenerate to session B and redeliver the era-A answer.
	regen <- struct{}{}
	signalSession.recvCh <- newHostedFlowAnswer(t, tpt.webrtcApi, ident.localPub, genA.offerSDP, genA.offerID)
	genB := awaitHostedFlowGeneration(t, gens)
	assertHostedFlowSuccessor(t, genA, genB)
	hold <- struct{}{}

	stale := awaitHostedFlowDelivery(t, deliveries)
	if stale.gen != genB.index || stale.body != "sdp/answer" || stale.err != nil {
		t.Fatalf("unexpected successor delivery for the stale answer: %+v", stale)
	}
	if stale.remoteDesc {
		t.Fatal("stale era-A answer touched the successor session's remote description")
	}
	if genB.sess.pc.RemoteDescription() != nil {
		t.Fatal("stale era-A answer left a remote description on the successor session")
	}
	if state := genB.sess.pc.SignalingState(); state != pion_webrtc.SignalingStateHaveLocalOffer {
		t.Fatalf("successor signaling state %v, want untouched HaveLocalOffer", state)
	}

	// Matching-generation material still delivers.
	signalSession.recvCh <- newHostedFlowAnswer(t, tpt.webrtcApi, ident.localPub, genB.offerSDP, genB.offerID)
	current := awaitHostedFlowDelivery(t, deliveries)
	if current.gen != genB.index || current.body != "sdp/answer" || current.err != nil || !current.remoteDesc {
		t.Fatalf("era-B answer did not establish on the successor session: %+v", current)
	}

	cancel()
	if err := <-resolverErr; err != context.Canceled {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}
}

// TestHostedFlowParkedMaterialNotReplayedIntoSuccessor pins that material
// parked against a retired tracker generation is dropped by the ingress fence
// and never replays into the successor session. If the trickle lands only
// after the successor was acquired, the session digest fence discards it
// before Pion instead; either way the successor's Pion state stays untouched.
func TestHostedFlowParkedMaterialNotReplayedIntoSuccessor(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	ident := newHostedFlowIdentity(t)
	logs := newHostedFlowLogs()
	tpt := newHostedFlowTransport(ctx, ident, logs)
	hold := make(chan struct{})
	regen := make(chan struct{})
	gens, deliveries := startHostedFlowTracker(t, tpt, ident.remotePeerID, hold, regen)

	signalSession := newHostedFlowSignalSession(ident, 3)
	resolverErr, _ := startHostedFlowResolver(ctx, tpt, signalSession)

	// Generation A never reads its signal stream: once the trickle's
	// delivery acquires the lease (proven by the generation publishing), any
	// decoded signal must park at the unbuffered execution hand-off. The tag
	// identifies a dead generation, so neither fence cares about its value.
	deadOfferID := make([]byte, 32)
	deadOfferID[0] = 0xab
	signalSession.recvCh <- newHostedFlowTrickle(t, ident.localPub, deadOfferID)
	genA := awaitHostedFlowGeneration(t, gens)

	// Retire A while the trickle is parked against it.
	regen <- struct{}{}
	parkedDrop := logs.awaitPrefix(t, "dropping stale-generation signal")
	if !parkedDrop && !logs.sawPrefix("dropping stale ice candidate") {
		t.Fatal("neither the ingress fence nor the session digest fence accounted for the parked trickle")
	}

	// The successor session receives only exempt marker traffic.
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	genB := awaitHostedFlowGeneration(t, gens)
	if genB.tracker == genA.tracker {
		t.Fatal("successor session reused the retired tracker")
	}
	hold <- struct{}{}

	markers := 0
	for markers < 2 {
		d := awaitHostedFlowDelivery(t, deliveries)
		if d.gen != genB.index {
			t.Fatalf("delivery from retired generation reached the report stream: %+v", d)
		}
		switch d.body {
		case "request_offer":
			if d.err != nil || d.remoteDesc || d.bufferedIce != 0 {
				t.Fatalf("marker delivery looked wrong: %+v", d)
			}
			markers++
		case "ice":
			if parkedDrop {
				t.Fatalf("retired-generation trickle replayed into the successor session: %+v", d)
			}
			if d.err != nil || d.remoteDesc || d.bufferedIce != 0 {
				t.Fatalf("late trickle touched the successor session before the digest fence dropped it: %+v", d)
			}
		default:
			t.Fatalf("unexpected successor delivery: %+v", d)
		}
	}
	select {
	case d := <-deliveries:
		t.Fatalf("extra successor delivery: %+v", d)
	default:
	}
	if genB.sess.pc.RemoteDescription() != nil {
		t.Fatal("successor session carried a remote description with no applied answer")
	}

	cancel()
	if err := <-resolverErr; err != context.Canceled {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}
}

// TestHostedFlowRequestOfferWakeCrossesFencedEra pins that an untagged
// description fences the peer's whole signal era, that later untagged ICE stays
// dropped, and that the exempt request_offer marker still wakes the successor
// session while fenced material never reaches it.
func TestHostedFlowRequestOfferWakeCrossesFencedEra(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	ident := newHostedFlowIdentity(t)
	logs := newHostedFlowLogs()
	tpt := newHostedFlowTransport(ctx, ident, logs)
	hold := make(chan struct{})
	regen := make(chan struct{})
	gens, deliveries := startHostedFlowTracker(t, tpt, ident.remotePeerID, hold, regen)

	signalSession := newHostedFlowSignalSession(ident, 5)
	resolverErr, _ := startHostedFlowResolver(ctx, tpt, signalSession)

	// Establish the lease with one exempt marker delivered to session A.
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	genA := awaitHostedFlowGeneration(t, gens)
	hold <- struct{}{}
	setup := awaitHostedFlowDelivery(t, deliveries)
	if setup.gen != genA.index || setup.body != "request_offer" || setup.err != nil {
		t.Fatalf("lease establishment marker failed: %+v", setup)
	}

	// An SDP description with no offer identity cannot be attributed to any
	// generation: the whole era fences and the description itself drops. The
	// marker behind it proves the resolver moved past the drop.
	signalSession.recvCh <- newHostedFlowUntaggedOffer(t, tpt.webrtcApi, ident.localPub)
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	first := awaitHostedFlowDelivery(t, deliveries)
	if first.gen != genA.index || first.body != "request_offer" || first.err != nil {
		t.Fatalf("untagged offer was not dropped before the marker: %+v", first)
	}

	// Era-fenced ICE stays dropped too; the next marker proves the fence acted.
	signalSession.recvCh <- newHostedFlowTrickle(t, ident.localPub, nil)
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	second := awaitHostedFlowDelivery(t, deliveries)
	if second.gen != genA.index || second.body != "request_offer" || second.err != nil {
		t.Fatalf("era-fenced ICE was not dropped: %+v", second)
	}

	// The exempt marker wakes the successor session across regeneration.
	regen <- struct{}{}
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	genB := awaitHostedFlowGeneration(t, gens)
	assertHostedFlowSuccessor(t, genA, genB)
	hold <- struct{}{}
	woke := awaitHostedFlowDelivery(t, deliveries)
	if woke.gen != genB.index || woke.body != "request_offer" || woke.err != nil {
		t.Fatalf("exempt marker did not wake the successor session: %+v", woke)
	}
	select {
	case d := <-deliveries:
		t.Fatalf("fenced material reached the successor session: %+v", d)
	default:
	}
	if genB.sess.pc.RemoteDescription() != nil {
		t.Fatal("successor session carried a remote description with no applied answer")
	}

	cancel()
	if err := <-resolverErr; err != context.Canceled {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}
}

// TestHostedFlowIngressDiscoverableAcrossRetirementGap pins that the peer's
// ingress lease stays discoverable between tracker generations: the resolver
// keeps its membership while the lease is detached, and the next delivery
// rebinds the successor tracker onto the same lease.
func TestHostedFlowIngressDiscoverableAcrossRetirementGap(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	ident := newHostedFlowIdentity(t)
	logs := newHostedFlowLogs()
	tpt := newHostedFlowTransport(ctx, ident, logs)
	hold := make(chan struct{})
	regen := make(chan struct{})
	gens, deliveries := startHostedFlowTracker(t, tpt, ident.remotePeerID, hold, regen)
	peerIDStr := ident.remotePeerID.String()

	signalSession := newHostedFlowSignalSession(ident, 2)
	resolverErr, resolver := startHostedFlowResolver(ctx, tpt, signalSession)

	// Establish the lease with one exempt marker delivered to session A.
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	genA := awaitHostedFlowGeneration(t, gens)
	hold <- struct{}{}
	first := awaitHostedFlowDelivery(t, deliveries)
	if first.gen != genA.index || first.body != "request_offer" || first.err != nil {
		t.Fatalf("lease establishment marker failed: %+v", first)
	}

	// Retire A: the lease must stay discoverable while detached.
	regen <- struct{}{}
	detached := waitForDetachedSignalIngress(t, tpt, peerIDStr, resolver)

	// The next delivery acquires the successor onto the same lease.
	signalSession.recvCh <- newHostedFlowMarker(t, ident.localPub)
	genB := awaitHostedFlowGeneration(t, gens)
	assertHostedFlowSuccessor(t, genA, genB)
	hold <- struct{}{}
	second := awaitHostedFlowDelivery(t, deliveries)
	if second.gen != genB.index || second.body != "request_offer" || second.err != nil {
		t.Fatalf("successor did not receive the wake marker: %+v", second)
	}

	var rebound *signalIngress
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		rebound = tpt.incomingSessions[peerIDStr]
	})
	if rebound == nil {
		t.Fatal("ingress lease disappeared after the successor acquisition")
	}
	if rebound != detached {
		t.Fatal("successor acquisition rebuilt the ingress lease instead of rebinding it")
	}
	if rebound.ref == nil || rebound.tracker != genB.tracker {
		t.Fatal("ingress did not rebind onto the successor tracker")
	}

	cancel()
	if err := <-resolverErr; err != context.Canceled {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}
}
