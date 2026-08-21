package webrtc

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
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

func waitForSignalIngress(
	t *testing.T,
	tpt *WebRTC,
	peerID string,
	resolver *handleSignalPeerResolver,
) *signalIngress {
	t.Helper()
	for {
		var member *signalIngress
		var waitCh <-chan struct{}
		tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if ingress := tpt.incomingSessions[peerID]; ingress != nil {
				if _, ok := ingress.resolvers[resolver]; ok {
					member = ingress
				}
			}
			waitCh = getWaitCh()
		})
		if member != nil {
			return member
		}
		<-waitCh
	}
}

func TestHandleSignalPeerRetriesRetiredTrackerSignal(t *testing.T) {
	testHandleSignalPeerRetriesRetiredTrackerSignal(t, false)
}

func TestHandleSignalPeerRetriesSignalAfterRoutineFailure(t *testing.T) {
	testHandleSignalPeerRetriesRetiredTrackerSignal(t, true)
}

func TestHandleSignalPeerRetriesSameTrackerExecutionGeneration(t *testing.T) {
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
	recvStarted := make(chan struct{}, 2)
	signalSession := &testSignalPeerSession{
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
		recvCh:       make(chan []byte, 1),
		recvStarted:  recvStarted,
	}
	signalSession.recvCh <- msg

	tpt := &WebRTC{
		le:               logrus.NewEntry(logrus.New()),
		conf:             &Config{},
		peerID:           localPeerID,
		privKey:          localPriv,
		incomingSessions: make(map[string]*signalIngress),
	}

	type executionDelivery struct {
		tracker   *sessionTracker
		execution *sessionTrackerExecution
		incoming  *incomingSignal
	}
	type executionResult struct {
		invocation int32
		err        error
	}

	firstReceived := make(chan executionDelivery, 1)
	secondReceived := make(chan executionDelivery, 1)
	secondStable := make(chan struct{})
	failFirst := make(chan struct{})
	unexpectedInvocation := make(chan int32, 1)
	routineDone := make(chan executionResult, 3)
	controlledErr := pkgerrors.New("controlled execution failure")
	var constructions atomic.Int32
	var invocations atomic.Int32
	var deliveries atomic.Int32

	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			constructions.Add(1)
			tkr := &sessionTracker{
				w:       tpt,
				le:      tpt.le,
				key:     key,
				peerID:  remotePeerID,
				offerer: true,
			}
			return func(ctx context.Context) (err error) {
				execution := tkr.beginExecution()
				invocation := invocations.Add(1)
				defer func() {
					tkr.retireExecution(execution)
					routineDone <- executionResult{invocation: invocation, err: err}
				}()

				var incoming *incomingSignal
				select {
				case <-ctx.Done():
					return context.Canceled
				case incoming = <-execution.rxSignal:
				}
				deliveries.Add(1)
				delivery := executionDelivery{
					tracker:   tkr,
					execution: execution,
					incoming:  incoming,
				}

				switch invocation {
				case 1:
					firstReceived <- delivery
					select {
					case <-ctx.Done():
						return context.Canceled
					case <-failFirst:
						return controlledErr
					}
				case 2:
					sess := &session{t: tkr}
					sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
						sess.acceptIncomingSignalLocked(incoming)
					})
					secondReceived <- delivery

					select {
					case <-ctx.Done():
						return context.Canceled
					case <-recvStarted:
						close(secondStable)
					case <-execution.rxSignal:
						deliveries.Add(1)
						return pkgerrors.New("received duplicate signal in one execution")
					}
					<-ctx.Done()
					return context.Canceled
				default:
					unexpectedInvocation <- invocation
					<-ctx.Done()
					return context.Canceled
				}
			}, tkr
		},
		keyed.WithBackoff[string, *sessionTracker](func(string) cbackoff.BackOff {
			return new(cbackoff.ZeroBackOff)
		}),
	)
	tpt.sessionTrackers.SetContext(ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)

	dialRef, dialTracker, existed := tpt.sessionTrackers.AddKeyRef(remotePeerIDStr)
	t.Cleanup(dialRef.Release)
	if existed {
		t.Fatal("dial reference unexpectedly found an existing tracker")
	}

	resolverErr := make(chan error, 1)
	resolver := &handleSignalPeerResolver{t: tpt, sess: signalSession}
	go func() {
		resolverErr <- resolver.Resolve(ctx, nil)
	}()

	<-recvStarted
	first := <-firstReceived
	if first.tracker != dialTracker {
		t.Fatal("first execution did not use the dial-held tracker")
	}
	close(failFirst)

	second := <-secondReceived
	<-secondStable
	if second.tracker != first.tracker {
		t.Fatal("WithBackoff replaced the sessionTracker pointer")
	}
	if second.execution.generation != first.execution.generation+1 {
		t.Fatalf(
			"execution generation advanced from %d to %d, want exactly one",
			first.execution.generation,
			second.execution.generation,
		)
	}
	if second.incoming != first.incoming {
		t.Fatal("resolver decoded or replaced the retained signal across restart")
	}
	select {
	case <-second.incoming.accepted:
	default:
		t.Fatal("restarted execution did not accept the retained signal")
	}
	if constructions.Load() != 1 {
		t.Fatalf("tracker constructions %d, want 1", constructions.Load())
	}
	if invocations.Load() != 2 {
		t.Fatalf("tracker executions %d, want 2", invocations.Load())
	}
	if deliveries.Load() != 2 {
		t.Fatalf("signal deliveries %d, want one per execution", deliveries.Load())
	}
	select {
	case invocation := <-unexpectedInvocation:
		t.Fatalf("unexpected tracker execution %d", invocation)
	default:
	}

	cancel()
	if err := <-resolverErr; err != context.Canceled {
		t.Fatalf("resolver returned %v, want context canceled", err)
	}

	results := make(map[int32]error, 2)
	for range 2 {
		result := <-routineDone
		results[result.invocation] = result.err
	}
	if results[1] != controlledErr {
		t.Fatalf("first execution returned %v, want %v", results[1], controlledErr)
	}
	if results[2] != context.Canceled {
		t.Fatalf("second execution returned %v, want context canceled", results[2])
	}

	dialRef.Release()
	if keys := tpt.sessionTrackers.GetKeys(); len(keys) != 0 {
		t.Fatalf("session trackers still registered after cancellation: %v", keys)
	}
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if dialTracker.execution != nil {
			t.Error("tracker execution remained published after cancellation")
		}
		if ingress := tpt.incomingSessions[remotePeerIDStr]; ingress != nil {
			t.Error("incoming session reference remained after cancellation")
		}
	})
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

	sig := &WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: 1}}
	msg, err := EncodeWebRtcSignal(sig, localPub)
	if err != nil {
		t.Fatal(err.Error())
	}
	firstRecvStarted := make(chan struct{}, 2)
	secondRecvStarted := make(chan struct{}, 1)
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
		recvStarted:  secondRecvStarted,
	}
	secondSession.recvCh <- msg

	tpt := &WebRTC{
		le:               logrus.NewEntry(logrus.New()),
		conf:             &Config{},
		peerID:           localPeerID,
		privKey:          localPriv,
		incomingSessions: make(map[string]*signalIngress),
	}

	firstReceived := make(chan *incomingSignal, 1)
	acceptFirst := make(chan struct{})
	trackerStable := make(chan struct{})
	trackerDone := make(chan error, 1)
	var deliveries atomic.Int32
	var generations atomic.Int32

	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			generations.Add(1)
			tkr := &sessionTracker{
				w:       tpt,
				le:      tpt.le,
				key:     key,
				peerID:  remotePeerID,
				offerer: true,
			}
			return func(ctx context.Context) (err error) {
				execution := tkr.beginExecution()
				defer tkr.retireExecution(execution)
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
				case incoming = <-execution.rxSignal:
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

				for {
					select {
					case next := <-execution.rxSignal:
						if next == incoming {
							deliveries.Add(1)
							sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
								sess.acceptIncomingSignalLocked(next)
							})
							return pkgerrors.New("received duplicate signal")
						}
						sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
							sess.acceptIncomingSignalLocked(next)
						})
					case <-firstRecvStarted:
						close(trackerStable)
						<-ctx.Done()
						return context.Canceled
					}
				}
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
	firstRef := waitForSignalIngress(t, tpt, remotePeerID.String(), firstResolver).ref

	secondCtx, cancelSecond := context.WithCancel(ctx)
	secondResolverErr := make(chan error, 1)
	secondResolver := &handleSignalPeerResolver{t: tpt, sess: secondSession}
	go func() {
		secondResolverErr <- secondResolver.Resolve(secondCtx, nil)
	}()

	<-secondRecvStarted
	secondRef := waitForSignalIngress(t, tpt, remotePeerID.String(), secondResolver).ref
	if secondRef != firstRef {
		t.Fatal("second resolver did not join the first resolver's ingress lease")
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

	cancelSecond()
	if err := <-secondResolverErr; err != context.Canceled {
		t.Fatalf("second resolver returned %v, want context canceled", err)
	}
	select {
	case err := <-trackerDone:
		t.Fatalf("tracker retired while the first resolver held membership: %v", err)
	default:
	}
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ingress := tpt.incomingSessions[remotePeerID.String()]
		if ingress == nil || ingress.ref != firstRef {
			t.Error("second resolver exit released the first resolver's ingress lease")
			return
		}
		if _, ok := ingress.resolvers[firstResolver]; !ok || len(ingress.resolvers) != 1 {
			t.Errorf("remaining ingress members = %d, want only the first resolver", len(ingress.resolvers))
		}
	})
	if t.Failed() {
		cancelFirst()
		return
	}

	cancelFirst()
	if err := <-firstResolverErr; err != context.Canceled {
		t.Fatalf("first resolver returned %v, want context canceled", err)
	}
	if err := <-trackerDone; err != context.Canceled {
		t.Fatalf("tracker returned %v, want context canceled", err)
	}
	if keys := tpt.sessionTrackers.GetKeys(); len(keys) != 0 {
		t.Fatalf("session trackers still registered after final resolver exit: %v", keys)
	}
}

func TestHandleSignalPeerReleasesOverlappingResolverReferences(t *testing.T) {
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
	secondRecvStarted := make(chan struct{}, 2)
	secondSession := &testSignalPeerSession{
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
		recvCh:       make(chan []byte, 1),
		recvStarted:  secondRecvStarted,
	}
	secondSession.recvCh <- msg

	tpt := &WebRTC{
		le:               logrus.NewEntry(logrus.New()),
		conf:             &Config{},
		peerID:           localPeerID,
		privKey:          localPriv,
		incomingSessions: make(map[string]*signalIngress),
	}

	firstReceived := make(chan *incomingSignal, 1)
	secondReceived := make(chan *incomingSignal, 1)
	trackerDone := make(chan error, 1)
	trackerContextDone := make(chan struct{})
	var tracker *sessionTracker
	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			tkr := &sessionTracker{
				w:       tpt,
				le:      tpt.le,
				key:     key,
				peerID:  remotePeerID,
				offerer: true,
			}
			tracker = tkr
			return func(ctx context.Context) (err error) {
				execution := tkr.beginExecution()
				defer func() {
					trackerDone <- err
				}()
				defer tkr.retireExecution(execution)

				var incoming *incomingSignal
				select {
				case <-ctx.Done():
					return context.Canceled
				case incoming = <-execution.rxSignal:
				}
				firstReceived <- incoming

				select {
				case <-ctx.Done():
					return context.Canceled
				case incoming = <-execution.rxSignal:
				}
				secondReceived <- incoming

				<-ctx.Done()
				close(trackerContextDone)
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
	firstRef := waitForSignalIngress(t, tpt, remotePeerIDStr, firstResolver).ref

	secondCtx, cancelSecond := context.WithCancel(ctx)
	secondResolverErr := make(chan error, 1)
	secondResolver := &handleSignalPeerResolver{t: tpt, sess: secondSession}
	go func() {
		secondResolverErr <- secondResolver.Resolve(secondCtx, nil)
	}()

	<-secondRecvStarted
	secondRef := waitForSignalIngress(t, tpt, remotePeerIDStr, secondResolver).ref
	firstLiveSession := &session{t: tracker}
	firstLiveSession.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		firstLiveSession.acceptIncomingSignalLocked(firstIncoming)
	})

	secondIncoming := <-secondReceived
	if secondRef == nil || secondRef != firstRef {
		t.Fatal("second resolver did not join the shared ingress lease")
	}
	secondLiveSession := &session{t: tracker}
	secondLiveSession.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		secondLiveSession.acceptIncomingSignalLocked(secondIncoming)
	})

	cancelSecond()
	if err := <-secondResolverErr; err != context.Canceled {
		t.Fatalf("second resolver returned %v, want context canceled", err)
	}
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ingress := tpt.incomingSessions[remotePeerIDStr]
		if ingress == nil || ingress.ref != firstRef {
			t.Error("second resolver exit released the first resolver's ingress lease")
			return
		}
		if _, ok := ingress.resolvers[firstResolver]; !ok || len(ingress.resolvers) != 1 {
			t.Errorf("remaining ingress members = %d, want only the first resolver", len(ingress.resolvers))
		}
	})
	if t.Failed() {
		cancelFirst()
		return
	}
	if keys := tpt.sessionTrackers.GetKeys(); len(keys) != 1 {
		t.Fatalf("session tracker released while the first resolver held membership: %v", keys)
	}

	cancelFirst()
	if err := <-firstResolverErr; err != context.Canceled {
		t.Fatalf("first resolver returned %v, want context canceled", err)
	}
	if ingress := tpt.incomingSessions[remotePeerIDStr]; ingress != nil {
		t.Fatal("final resolver release retained the ingress lease, want nil")
	}
	if keys := tpt.sessionTrackers.GetKeys(); len(keys) != 0 {
		t.Fatalf("session tracker remained after final resolver release: %v", keys)
	}
	<-trackerContextDone
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
		incomingSessions: make(map[string]*signalIngress),
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
				w:       tpt,
				le:      tpt.le,
				key:     key,
				peerID:  remotePeerID,
				offerer: true,
			}
			return func(ctx context.Context) error {
				execution := tkr.beginExecution()
				defer tkr.retireExecution(execution)
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
				case incoming = <-execution.rxSignal:
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

					tkr.retireExecution(execution)
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

var _ signaling.SignalPeerSession = (*testSignalPeerSession)(nil)

// TestHandleSignalPeerReacquiresAfterTrackerExecutionRetires pins the
// reacquisition contract: a resolver whose unaccepted signal was left on a
// retired tracker execution must reacquire a lease and redeliver that signal
// to a fresh tracker, instead of parking forever with no tracker live.
func TestHandleSignalPeerReacquiresAfterTrackerExecutionRetires(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	localPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
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
	peerKey := remotePeerID.String()

	tpt := &WebRTC{
		le:               logrus.NewEntry(logrus.New()),
		conf:             &Config{},
		peerID:           localPeerID,
		privKey:          localPriv,
		incomingSessions: make(map[string]*signalIngress),
	}

	firstDelivered := make(chan *incomingSignal, 1)
	var constructions atomic.Int32
	var firstTracker *sessionTracker
	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			construction := constructions.Add(1)
			tkr := &sessionTracker{w: tpt, le: tpt.le, key: key}
			if construction == 1 {
				firstTracker = tkr
			}
			return func(ctx context.Context) error {
				execution := tkr.beginExecution()
				defer tkr.retireExecution(execution)
				select {
				case <-ctx.Done():
					return context.Canceled
				case incoming := <-execution.rxSignal:
					if construction == 1 {
						// Receive without accepting; the test retires this
						// ingress while the signal is still pending.
						firstDelivered <- incoming
						<-ctx.Done()
						return context.Canceled
					}
					sess := &session{t: tkr}
					sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
						sess.acceptIncomingSignalLocked(incoming)
					})
					<-ctx.Done()
					return context.Canceled
				}
			}, tkr
		},
		keyed.WithBackoff[string, *sessionTracker](func(string) cbackoff.BackOff {
			return new(cbackoff.ZeroBackOff)
		}),
	)
	tpt.sessionTrackers.SetContext(ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)

	resolverA := &handleSignalPeerResolver{t: tpt}
	resolverB := &handleSignalPeerResolver{t: tpt}
	incoming := &incomingSignal{accepted: make(chan struct{})}

	deliverErr := make(chan error, 1)
	go func() {
		deliverErr <- tpt.deliverSignal(ctx, peerKey, resolverA, incoming)
	}()

	// Resolver A acquires the first ingress and delivers without acceptance.
	<-firstDelivered

	// Resolver B joins A's lease on the same live tracker.
	var sharedTracker *sessionTracker
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ingress, err := tpt.acquireSignalIngressLocked(peerKey, resolverB, broadcast)
		if err != nil {
			t.Errorf("join shared ingress: %v", err)
			return
		}
		sharedTracker = ingress.tracker
	})
	if t.Failed() {
		return
	}

	// Publish a fresh execution generation on the shared tracker, then
	// receive A's redelivery on it. Both resolvers remain members of the
	// ingress until the execution retires.
	replacementExecution := firstTracker.beginExecution()
	redelivered := <-replacementExecution.rxSignal
	if redelivered != incoming {
		t.Fatal("redelivery carried a different signal")
	}

	// The tracker execution retires before A's signal is accepted, leaving
	// the peer with no ingress lease.
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		tpt.retireSignalIngressLocked(peerKey, sharedTracker)
		broadcast()
	})

	// A reacquires a fresh tracker and redelivers until acceptance.
	if err := <-deliverErr; err != nil {
		t.Fatalf("deliverSignal returned %v, want nil after reacquisition", err)
	}
	select {
	case <-incoming.accepted:
	default:
		t.Fatal("signal was not accepted after reacquisition")
	}
	if got := constructions.Load(); got != 2 {
		t.Fatalf("tracker constructions %d, want 2 (original plus reacquired)", got)
	}
}

func TestHandleSignalPeerResolverExitPreservesSharedIngress(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	localPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
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
	peerKey := remotePeerID.String()

	tpt := &WebRTC{
		le:               logrus.NewEntry(logrus.New()),
		conf:             &Config{},
		peerID:           localPeerID,
		privKey:          localPriv,
		incomingSessions: make(map[string]*signalIngress),
	}

	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			tkr := &sessionTracker{w: tpt, le: tpt.le, key: key}
			return func(ctx context.Context) error {
				execution := tkr.beginExecution()
				defer tkr.retireExecution(execution)
				sess := &session{t: tkr}
				for {
					select {
					case <-ctx.Done():
						return context.Canceled
					case incoming := <-execution.rxSignal:
						sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
							sess.acceptIncomingSignalLocked(incoming)
						})
					}
				}
			}, tkr
		},
		keyed.WithBackoff[string, *sessionTracker](func(string) cbackoff.BackOff {
			return new(cbackoff.ZeroBackOff)
		}),
	)
	tpt.sessionTrackers.SetContext(ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)

	resolverA := &handleSignalPeerResolver{t: tpt}
	resolverB := &handleSignalPeerResolver{t: tpt}

	// Resolver A delivers its first signal and holds the lease.
	first := &incomingSignal{accepted: make(chan struct{})}
	if err := tpt.deliverSignal(ctx, peerKey, resolverA, first); err != nil {
		t.Fatalf("first delivery returned %v", err)
	}

	// Resolver B joins while A's negotiation is active.
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if _, err := tpt.acquireSignalIngressLocked(peerKey, resolverB, broadcast); err != nil {
			t.Errorf("join shared ingress: %v", err)
		}
	})
	if t.Failed() {
		return
	}

	var sharedTracker *sessionTracker
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ingress := tpt.incomingSessions[peerKey]
		if ingress == nil || len(ingress.resolvers) != 2 {
			t.Errorf("ingress members = %d, want 2", len(ingress.resolvers))
			return
		}
		sharedTracker = ingress.tracker
	})
	if t.Failed() {
		return
	}

	// A exits. B keeps the tracker execution and its negotiation alive.
	tpt.closeSignalIngress(peerKey, resolverA)

	second := &incomingSignal{
		sig:      &WebRtcSignal{Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{}}},
		accepted: make(chan struct{}),
	}
	if err := tpt.deliverSignal(ctx, peerKey, resolverB, second); err != nil {
		t.Fatalf("second delivery returned %v", err)
	}
	select {
	case <-second.accepted:
	default:
		t.Fatal("second signal was not accepted")
	}

	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ingress := tpt.incomingSessions[peerKey]
		if ingress == nil || ingress.tracker != sharedTracker {
			t.Error("resolver exit replaced the shared tracker")
			return
		}
		if _, ok := ingress.resolvers[resolverB]; !ok || len(ingress.resolvers) != 1 {
			t.Errorf("remaining ingress members = %d, want only resolver B", len(ingress.resolvers))
		}
	})

	stale := &incomingSignal{accepted: make(chan struct{})}
	if err := tpt.deliverSignal(ctx, peerKey, resolverA, stale); !errors.Is(err, context.Canceled) {
		t.Fatalf("stale resolver delivery returned %v, want context canceled", err)
	}
}

// TestHandleSignalPeerJoinsMidHandshakeWithoutReplacement pins the
// mid-handshake contract: a second signaling session joins the live peer
// tracker while an offer is pending instead of canceling or replacing it,
// and completes the negotiation on the same tracker execution even after
// the first signaling session exits.
func TestHandleSignalPeerJoinsMidHandshakeWithoutReplacement(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)

	localPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
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
	peerKey := remotePeerID.String()

	tpt := &WebRTC{
		le:               logrus.NewEntry(logrus.New()),
		conf:             &Config{},
		peerID:           localPeerID,
		privKey:          localPriv,
		incomingSessions: make(map[string]*signalIngress),
	}

	var constructions atomic.Int32
	var invocations atomic.Int32
	offerPending := make(chan struct{})
	pairAccepted := make(chan struct{})
	tpt.sessionTrackers = keyed.NewKeyedRefCount(
		func(key string) (keyed.Routine, *sessionTracker) {
			constructions.Add(1)
			tkr := &sessionTracker{w: tpt, le: tpt.le, key: key}
			return func(ctx context.Context) error {
				invocations.Add(1)
				execution := tkr.beginExecution()
				defer tkr.retireExecution(execution)
				sess := &session{t: tkr}
				received := 0
				var pending *incomingSignal
				for {
					select {
					case <-ctx.Done():
						return context.Canceled
					case incoming := <-execution.rxSignal:
						received++
						switch received {
						case 1:
							// Hold the offer pending, unaccepted, exactly
							// like a live negotiation between offer and
							// answer.
							pending = incoming
							offerPending <- struct{}{}
						case 2:
							sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
								sess.acceptIncomingSignalLocked(pending)
								sess.acceptIncomingSignalLocked(incoming)
							})
							close(pairAccepted)
						default:
							sess.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
								sess.acceptIncomingSignalLocked(incoming)
							})
						}
					}
				}
			}, tkr
		},
		keyed.WithBackoff[string, *sessionTracker](func(string) cbackoff.BackOff {
			return new(cbackoff.ZeroBackOff)
		}),
	)
	tpt.sessionTrackers.SetContext(ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)

	resolverA := &handleSignalPeerResolver{t: tpt}
	resolverB := &handleSignalPeerResolver{t: tpt}

	deliverErrA := make(chan error, 1)
	go func() {
		deliverErrA <- tpt.deliverSignal(ctx, peerKey, resolverA, &incomingSignal{
			sig:      &WebRtcSignal{Body: &WebRtcSignal_RequestOffer{RequestOffer: 1}},
			accepted: make(chan struct{}),
		})
	}()

	<-offerPending
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ingress := tpt.incomingSessions[peerKey]
		if ingress == nil || len(ingress.resolvers) != 1 {
			t.Errorf("pending-offer ingress members = %d, want 1", len(ingress.resolvers))
		}
	})

	// The second signaling session arrives mid-handshake with the answer.
	deliverErrB := make(chan error, 1)
	go func() {
		deliverErrB <- tpt.deliverSignal(ctx, peerKey, resolverB, &incomingSignal{
			sig:      &WebRtcSignal{Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{}}},
			accepted: make(chan struct{}),
		})
	}()

	select {
	case <-pairAccepted:
	case <-time.After(10 * time.Second):
		t.Fatal("tracker never paired the pending offer with the joined answer")
	}
	if err := <-deliverErrA; err != nil {
		t.Fatalf("first delivery returned %v", err)
	}
	if err := <-deliverErrB; err != nil {
		t.Fatalf("joined delivery returned %v", err)
	}
	if got := constructions.Load(); got != 1 {
		t.Fatalf("tracker constructions %d, want 1 (no cancel/recreate mid-handshake)", got)
	}
	if got := invocations.Load(); got != 1 {
		t.Fatalf("tracker executions %d, want 1 (no execution replacement mid-handshake)", got)
	}

	// The first signaling session exits mid-negotiation; the joined session
	// keeps the same tracker execution and finishes ICE on it.
	tpt.closeSignalIngress(peerKey, resolverA)
	ice := &incomingSignal{
		sig:      &WebRtcSignal{Body: &WebRtcSignal_Sdp{Sdp: &WebRtcSdp{}}},
		accepted: make(chan struct{}),
	}
	if err := tpt.deliverSignal(ctx, peerKey, resolverB, ice); err != nil {
		t.Fatalf("post-exit delivery returned %v", err)
	}
	select {
	case <-ice.accepted:
	default:
		t.Fatal("ICE signal was not accepted")
	}
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		ingress := tpt.incomingSessions[peerKey]
		if ingress == nil || len(ingress.resolvers) != 1 {
			t.Errorf("remaining ingress members = %d, want only the joined resolver", len(ingress.resolvers))
			return
		}
		if _, ok := ingress.resolvers[resolverB]; !ok {
			t.Error("joined resolver lost its ingress membership")
		}
	})

	stale := &incomingSignal{accepted: make(chan struct{})}
	if err := tpt.deliverSignal(ctx, peerKey, resolverA, stale); !errors.Is(err, context.Canceled) {
		t.Fatalf("exited resolver delivery returned %v, want context canceled", err)
	}
}
