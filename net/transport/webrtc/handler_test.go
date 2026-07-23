package webrtc

import (
	"bytes"
	"context"
	"crypto/rand"
	"sync/atomic"
	"testing"

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
		incomingSessions: make(map[string]*keyed.KeyedRef[string, *sessionTracker]),
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
		if ref := tpt.incomingSessions[remotePeerIDStr]; ref != nil {
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
	var firstRef *keyed.KeyedRef[string, *sessionTracker]
	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		firstRef = tpt.incomingSessions[remotePeerIDStr]
	})
	if firstRef == nil {
		t.Fatal("first resolver did not install its incoming session reference")
	}

	replacementReady := make(chan struct{})
	continueReplacement := make(chan struct{})
	secondCtx, cancelSecond := context.WithCancel(ctx)
	secondResolverErr := make(chan error, 1)
	secondResolver := &handleSignalPeerResolver{
		t:    tpt,
		sess: secondSession,
		refStored: func(
			prev *keyed.KeyedRef[string, *sessionTracker],
			next *keyed.KeyedRef[string, *sessionTracker],
		) {
			if prev == firstRef {
				close(replacementReady)
				<-continueReplacement
			}
		},
	}
	go func() {
		secondResolverErr <- secondResolver.Resolve(secondCtx, nil)
	}()

	<-replacementReady
	close(acceptFirst)
	close(continueReplacement)
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
	if err := <-trackerDone; err != context.Canceled {
		t.Fatalf("tracker returned %v, want context canceled", err)
	}
	if keys := tpt.sessionTrackers.GetKeys(); len(keys) != 0 {
		t.Fatalf("session trackers still registered after overlapping resolver exit: %v", keys)
	}

	cancelFirst()
	if err := <-firstResolverErr; err != context.Canceled {
		t.Fatalf("first resolver returned %v, want context canceled", err)
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
		incomingSessions: make(map[string]*keyed.KeyedRef[string, *sessionTracker]),
	}

	firstReceived := make(chan *incomingSignal, 1)
	secondReceived := make(chan *incomingSignal, 1)
	retireTracker := make(chan struct{})
	trackerRetired := make(chan struct{})
	trackerDone := make(chan error, 1)
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
				defer tkr.retireExecution(execution)
				defer func() {
					trackerDone <- err
				}()

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

				select {
				case <-ctx.Done():
					return context.Canceled
				case <-retireTracker:
				}
				tkr.retireExecution(execution)
				close(trackerRetired)
				return nil
			}, tkr
		},
	)
	tpt.sessionTrackers.SetContext(ctx, true)
	t.Cleanup(tpt.sessionTrackers.ClearContext)

	releaseCalls := make(chan *keyed.KeyedRef[string, *sessionTracker], 4)
	refReplaced := make(chan [2]*keyed.KeyedRef[string, *sessionTracker], 1)
	continueReplacement := make(chan struct{})
	releaseRef := func(ref *keyed.KeyedRef[string, *sessionTracker]) {
		releaseCalls <- ref
		ref.Release()
	}
	refStored := func(
		prev *keyed.KeyedRef[string, *sessionTracker],
		next *keyed.KeyedRef[string, *sessionTracker],
	) {
		if prev != nil {
			refReplaced <- [2]*keyed.KeyedRef[string, *sessionTracker]{prev, next}
			<-continueReplacement
		}
	}

	firstCtx, cancelFirst := context.WithCancel(ctx)
	firstResolverErr := make(chan error, 1)
	firstResolver := &handleSignalPeerResolver{
		t:          tpt,
		sess:       firstSession,
		releaseRef: releaseRef,
		refStored:  refStored,
	}
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
	secondResolver := &handleSignalPeerResolver{
		t:          tpt,
		sess:       secondSession,
		releaseRef: releaseRef,
		refStored:  refStored,
	}
	go func() {
		secondResolverErr <- secondResolver.Resolve(secondCtx, nil)
	}()

	<-secondRecvStarted
	replaced := <-refReplaced
	if replaced[0] != firstRef {
		close(continueReplacement)
		t.Fatal("overlapping resolver replaced an unexpected reference")
	}
	select {
	case released := <-releaseCalls:
		if released != firstRef {
			close(continueReplacement)
			t.Fatal("overlapping resolver released an unexpected reference")
		}
	default:
		close(continueReplacement)
		t.Fatal("overlapping resolver did not release the superseded reference")
	}

	firstLiveSession := &session{t: tracker}
	firstLiveSession.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		firstLiveSession.acceptIncomingSignalLocked(firstIncoming)
	})
	close(continueReplacement)

	secondIncoming := <-secondReceived
	secondRef := replaced[1]
	if secondRef == nil || secondRef == firstRef {
		t.Fatal("second resolver did not install a distinct incoming session reference")
	}
	secondLiveSession := &session{t: tracker}
	secondLiveSession.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		secondLiveSession.acceptIncomingSignalLocked(secondIncoming)
	})

	<-firstRecvStarted
	<-secondRecvStarted
	close(retireTracker)
	<-trackerRetired
	if err := <-trackerDone; err != nil {
		t.Fatalf("tracker returned %v, want nil", err)
	}

	tpt.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if ref := tpt.incomingSessions[remotePeerIDStr]; ref != nil {
			t.Errorf("incoming session reference remained after retirement: %p", ref)
		}
	})
	if keys := tpt.sessionTrackers.GetKeys(); len(keys) != 0 {
		t.Fatalf("session trackers still registered after both references released: %v", keys)
	}

	cancelFirst()
	if err := <-firstResolverErr; err != context.Canceled {
		t.Fatalf("first resolver returned %v, want context canceled", err)
	}
	cancelSecond()
	if err := <-secondResolverErr; err != context.Canceled {
		t.Fatalf("second resolver returned %v, want context canceled", err)
	}

	select {
	case released := <-releaseCalls:
		if released == secondRef {
			t.Fatal("resolver released the reference already released by tracker retirement")
		}
		t.Fatalf("resolver released superseded reference %p more than once", released)
	default:
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

var _ signaling.SignalPeerSession = ((*testSignalPeerSession)(nil))
