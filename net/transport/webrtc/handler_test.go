package webrtc

import (
	"bytes"
	"context"
	"crypto/rand"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/util/keyed"
	pion_webrtc "github.com/pion/webrtc/v4"
	pkgerrors "github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	"github.com/sirupsen/logrus"
)

type testSignalPeerSession struct {
	localPeerID  peer.ID
	remotePeerID peer.ID
	recvCh       chan []byte
	recvStarted  chan struct{}
}

func (s *testSignalPeerSession) GetLocalPeerID() peer.ID {
	return s.localPeerID
}

func (s *testSignalPeerSession) GetRemotePeerID() peer.ID {
	return s.remotePeerID
}

func (s *testSignalPeerSession) Send(context.Context, []byte) error {
	return nil
}

func (s *testSignalPeerSession) Recv(ctx context.Context) ([]byte, error) {
	if s.recvStarted != nil {
		s.recvStarted <- struct{}{}
	}
	select {
	case <-ctx.Done():
		return nil, context.Canceled
	case msg := <-s.recvCh:
		return msg, nil
	}
}

func TestHandleSignalPeerRetriesRetiredTrackerSignal(t *testing.T) {
	testHandleSignalPeerRetriesRetiredTrackerSignal(t, false)
}

func TestHandleSignalPeerRetriesSignalAfterRoutineFailure(t *testing.T) {
	testHandleSignalPeerRetriesRetiredTrackerSignal(t, true)
}

func TestHandleSignalPeerDeliversOncePerTrackerGeneration(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

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
	remotePeerIDStr := remotePeerID.String()

	sig := &WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: 1}}
	msg, err := EncodeWebRtcSignal(sig, localPub)
	if err != nil {
		t.Fatal(err.Error())
	}
	firstRecvStarted := make(chan struct{}, 2)
	firstSession := &testSignalPeerSession{
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
		recvCh:       make(chan []byte, 1),
		recvStarted:  firstRecvStarted,
	}
	firstSession.recvCh <- bytes.Clone(msg)
	secondSession := &testSignalPeerSession{
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
		recvCh:       make(chan []byte, 1),
	}
	secondSession.recvCh <- msg

	tpt := &WebRTC{
		le:               logrus.NewEntry(logrus.New()),
		conf:             &Config{},
		peerID:           localPeerID,
		privKey:          localPriv,
		incomingSessions: make(map[string]*keyed.KeyedRef[string, *sessionTracker]),
	}

	firstReceived := make(chan *incomingSignal, 1)
	acceptFirst := make(chan struct{})
	trackerStable := make(chan struct{})
	trackerDone := make(chan error, 1)
	var deliveries atomic.Int32
	var generations atomic.Int32
	var generation *sessionTracker

	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			generations.Add(1)
			tkr := &sessionTracker{
				w:        tpt,
				le:       tpt.le,
				key:      key,
				peerID:   remotePeerID,
				offerer:  true,
				rxSignal: make(chan *incomingSignal),
			}
			generation = tkr
			return func(ctx context.Context) (err error) {
				defer func() {
					if rec := recover(); rec != nil {
						err = pkgerrors.Errorf("tracker panicked: %v", rec)
					}
					trackerDone <- err
				}()

				var incoming *incomingSignal
				select {
				case <-ctx.Done():
					return context.Canceled
				case incoming = <-tkr.rxSignal:
				}
				deliveries.Add(1)
				firstReceived <- incoming

				select {
				case <-ctx.Done():
					return context.Canceled
				case <-acceptFirst:
				}
				sess := &session{t: tkr}
				sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
					sess.acceptIncomingSignalLocked(incoming)
				})

				select {
				case duplicate := <-tkr.rxSignal:
					deliveries.Add(1)
					sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
						sess.acceptIncomingSignalLocked(duplicate)
					})
					return pkgerrors.New("received duplicate signal")
				case <-firstRecvStarted:
				}
				close(trackerStable)

				<-ctx.Done()
				return context.Canceled
			}, tkr
		},
	)
	tpt.sessionTrackers.SetContext(ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)

	firstCtx, cancelFirst := context.WithCancel(ctx)
	firstResolverErr := make(chan error, 1)
	firstResolver := &handleSignalPeerResolver{t: tpt, sess: firstSession}
	go func() {
		firstResolverErr <- firstResolver.Resolve(firstCtx, nil)
	}()

	<-firstRecvStarted
	firstIncoming := <-firstReceived
	var firstRef *keyed.KeyedRef[string, *sessionTracker]
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		firstRef = tpt.incomingSessions[remotePeerIDStr]
	})
	if firstRef == nil {
		t.Fatal("first resolver did not install its incoming session reference")
	}

	secondCtx, cancelSecond := context.WithCancel(ctx)
	secondResolverErr := make(chan error, 1)
	secondResolver := &handleSignalPeerResolver{t: tpt, sess: secondSession}
	go func() {
		secondResolverErr <- secondResolver.Resolve(secondCtx, nil)
	}()

	for {
		var currRef *keyed.KeyedRef[string, *sessionTracker]
		var waitCh <-chan struct{}
		tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			currRef = tpt.incomingSessions[remotePeerIDStr]
			waitCh = getWaitCh()
		})
		if currRef != nil && currRef != firstRef {
			break
		}
		select {
		case err := <-secondResolverErr:
			t.Fatalf("second resolver returned before replacing the shared reference: %v", err)
		case <-waitCh:
		}
	}
	cancelSecond()
	if err := <-secondResolverErr; err != context.Canceled {
		t.Fatalf("second resolver returned %v, want context canceled", err)
	}

	for {
		var currRef *keyed.KeyedRef[string, *sessionTracker]
		var waitCh <-chan struct{}
		tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			currRef = tpt.incomingSessions[remotePeerIDStr]
			waitCh = getWaitCh()
		})
		if currRef != nil && currRef != firstRef {
			break
		}
		<-waitCh
	}

	close(acceptFirst)
	select {
	case <-trackerStable:
	case err := <-trackerDone:
		t.Fatalf("tracker retired during overlapping delivery: %v", err)
	}

	select {
	case <-firstIncoming.accepted:
	default:
		t.Fatal("tracker did not accept the first incoming signal")
	}
	if deliveries.Load() != 1 {
		t.Fatalf("signal deliveries %d, want 1", deliveries.Load())
	}
	if generations.Load() != 1 {
		t.Fatalf("tracker generations %d, want 1", generations.Load())
	}
	ref, tkr, existed := tpt.sessionTrackers.AddKeyRef(remotePeerIDStr)
	ref.Release()
	if !existed {
		t.Fatal("accepted tracker generation retired")
	}
	if tkr != generation {
		t.Fatal("keyed reference did not retain the accepted tracker generation")
	}
	select {
	case err := <-trackerDone:
		t.Fatalf("tracker exited before resolver shutdown: %v", err)
	default:
	}

	cancelFirst()
	if err := <-firstResolverErr; err != context.Canceled {
		t.Fatalf("first resolver returned %v, want context canceled", err)
	}
	cancel()
	if err := <-trackerDone; err != context.Canceled {
		t.Fatalf("tracker returned %v, want context canceled", err)
	}
}

func testHandleSignalPeerRetriesRetiredTrackerSignal(t *testing.T, routineFailure bool) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

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

	sig := &WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: 1}}
	msg, err := EncodeWebRtcSignal(sig, localPub)
	if err != nil {
		t.Fatal(err.Error())
	}
	signalSession := &testSignalPeerSession{
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
		recvCh:       make(chan []byte, 1),
	}
	signalSession.recvCh <- msg

	tpt := &WebRTC{
		le:               logrus.NewEntry(logrus.New()),
		conf:             &Config{},
		peerID:           localPeerID,
		privKey:          localPriv,
		incomingSessions: make(map[string]*keyed.KeyedRef[string, *sessionTracker]),
	}

	type replacementResult struct {
		incoming *incomingSignal
		offers   []*WebRtcSignal
		err      error
	}
	oldReceived := make(chan *incomingSignal, 1)
	retireOld := make(chan struct{})
	replacementDone := make(chan replacementResult, 1)
	oldRetired := make(chan error, 1)
	acceptReplacement := make(chan struct{})
	var generations atomic.Int32

	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			generation := generations.Add(1)
			tkr := &sessionTracker{
				w:        tpt,
				le:       tpt.le,
				key:      key,
				peerID:   remotePeerID,
				offerer:  true,
				rxSignal: make(chan *incomingSignal),
			}
			return func(ctx context.Context) error {
				var oldSession *session
				var routineErr error
				var routineErrCh chan error
				if generation == 1 {
					oldSession = &session{t: tkr}
					if routineFailure {
						// Make the routine error observable before rxSignal. The receive
						// below deliberately takes the hazardous signal-first branch.
						routineErrCh = make(chan error, 1)
						routineErr = pkgerrors.New("controlled routine failure")
						oldSession.failWithErr(routineErrCh, routineErr)
					}
				}

				var incoming *incomingSignal
				select {
				case <-ctx.Done():
					return context.Canceled
				case incoming = <-tkr.rxSignal:
				}

				if generation == 1 {
					oldReceived <- incoming
					<-retireOld

					oldSession.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
						if !routineFailure {
							oldSession.connState = pion_webrtc.PeerConnectionStateFailed
						}
						oldSession.acceptIncomingSignalLocked(incoming)
					})

					var retireErr error
					if routineFailure {
						if err := <-routineErrCh; err != routineErr {
							retireErr = pkgerrors.Errorf(
								"terminal error %v, want %v",
								err,
								routineErr,
							)
						}
					}

					tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
						if ref := tpt.incomingSessions[key]; ref != nil {
							ref.Release()
							delete(tpt.incomingSessions, key)
							broadcast()
						}
					})
					oldRetired <- retireErr
					return nil
				}

				select {
				case <-ctx.Done():
					return context.Canceled
				case <-acceptReplacement:
				}

				replacementSession := &session{t: tkr}
				replacementSession.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
					replacementSession.localSeqno = 1
					replacementSession.acceptIncomingSignalLocked(incoming)
				})

				pc, err := pion_webrtc.NewPeerConnection(pion_webrtc.Configuration{})
				if err != nil {
					replacementDone <- replacementResult{incoming: incoming, err: err}
					return err
				}
				if _, err := pc.CreateDataChannel(dataChannelID, nil); err != nil {
					if closeErr := pc.Close(); closeErr != nil {
						err = pkgerrors.Wrapf(err, "close peer connection: %v", closeErr)
					}
					replacementDone <- replacementResult{incoming: incoming, err: err}
					return err
				}
				replacementSession.pc = pc

				var offers []*WebRtcSignal
				_, transmitted, err := tkr.transmitLocalNegotiation(
					replacementSession,
					tkr.le,
					1,
					0,
					func(sig *WebRtcSignal) {
						offers = append(offers, sig)
					},
				)
				if err == nil && !transmitted {
					err = pkgerrors.New("replacement did not transmit an offer")
				}
				if closeErr := pc.Close(); err == nil && closeErr != nil {
					err = closeErr
				}
				replacementDone <- replacementResult{
					incoming: incoming,
					offers:   offers,
					err:      err,
				}
				return err
			}, tkr
		},
	)
	tpt.sessionTrackers.SetContext(ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)

	resolverErr := make(chan error, 1)
	resolver := &handleSignalPeerResolver{t: tpt, sess: signalSession}
	go func() {
		resolverErr <- resolver.Resolve(ctx, nil)
	}()

	oldIncoming := <-oldReceived
	select {
	case <-oldIncoming.accepted:
		t.Fatal("retiring tracker accepted the signal before retirement was released")
	default:
	}
	close(retireOld)
	if err := <-oldRetired; err != nil {
		t.Fatal(err.Error())
	}
	select {
	case <-oldIncoming.accepted:
		t.Fatal("failed tracker accepted the signal")
	default:
	}
	close(acceptReplacement)

	replacement := <-replacementDone
	if replacement.err != nil {
		t.Fatal(replacement.err.Error())
	}
	replacementIncoming := replacement.incoming
	if replacementIncoming != oldIncoming {
		t.Fatal("handler decoded or replaced the signal during generation handoff")
	}
	if replacementIncoming.sig.GetRequestOffer() != 1 {
		t.Fatalf("replacement received request_offer %d, want 1", replacementIncoming.sig.GetRequestOffer())
	}
	select {
	case <-replacementIncoming.accepted:
	default:
		t.Fatal("replacement tracker did not positively accept the signal")
	}
	if len(replacement.offers) != 1 {
		t.Fatalf("offer count %d, want 1", len(replacement.offers))
	}
	if replacement.offers[0].GetSdp().GetSdpType() != "offer" {
		t.Fatalf("replacement emitted %q, want offer", replacement.offers[0].GetSdp().GetSdpType())
	}
	if generations.Load() != 2 {
		t.Fatalf("tracker generations %d, want 2", generations.Load())
	}

	cancel()
	if err := <-resolverErr; err != context.Canceled {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}
}

var _ signaling.SignalPeerSession = ((*testSignalPeerSession)(nil))
