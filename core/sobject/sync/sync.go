package sobject_sync

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/crypto"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// SyncProtocolID is the protocol ID used for SO sync solicitation.
const SyncProtocolID = protocol.ID("alpha/so-sync/2")

const (
	// maxMessageSize is the max message size for SO sync messages. A snapshot
	// carries one whole state.
	maxMessageSize = sobject.MaxSyncStateSize
	// syncRetryInitialInterval is the first delay after a recoverable stream
	// failure.
	syncRetryInitialInterval = 250 * time.Millisecond
	// syncRetryMaxInterval caps repeated recoverable stream-failure delays.
	syncRetryMaxInterval = 2 * time.Second
	// syncRetryMultiplier doubles the delay until syncRetryMaxInterval.
	syncRetryMultiplier = 2
)

// SnapshotAccessValidator verifies that the local object identity can decode
// an inbound snapshot before it replaces durable state.
type SnapshotAccessValidator func(context.Context, *sobject.SOState) error

// SOSync synchronizes one shared object over the session transport's child bus.
type SOSync struct {
	// le records synchronization errors and peer activity.
	le *logrus.Entry
	// b supplies the session transport's solicitation interface.
	b bus.Bus
	// soID identifies the object and its signature context.
	soID string
	// localObjectPeerID is the participant identity, independent of transport
	// identity.
	localObjectPeerID peer.ID
	// localObjectKey proves possession of the participant identity.
	localObjectKey crypto.PrivKey
	// soHost owns accepted state and its provider lock.
	soHost *sobject.SOHost
	// validateSnapshotAccess checks local decryption before acceptance.
	validateSnapshotAccess SnapshotAccessValidator
	// peerAdmission reports a locally permitted participant's explicit admission
	// response.
	peerAdmission func(peer.ID, bool)
	// peerRecovery reports trusted recovery requirements independently of admission.
	peerRecovery func(peer.ID, bool)
	// failures remembers the last logged stream failure of each peer.
	failures syncFailures
}

// solicitationGeneration owns the workers admitted by one solicitation
// directive lifetime and publishes the first recoverable worker failure.
type solicitationGeneration struct {
	// bcast guards stopping, workers, and retryErr.
	bcast broadcast.Broadcast
	// stopping prevents new workers while the generation drains.
	stopping bool
	// workers counts admitted workers that have not returned.
	workers int
	// retryErr is the first recoverable worker failure.
	retryErr error
}

// NewSOSync constructs a new SOSync.
//
// localObjectPeerID is the local storage identity checked against inbound
// state. The transport peer routes the sync stream but need not be a Space
// participant. localObjectKey must belong to localObjectPeerID and remains
// available for the SOSync lifetime. Authentication rejects a mismatched key.
// peerAdmission, when non-nil, observes explicit responses from peers permitted
// by local authority. Calls may be concurrent and must return promptly. A remote
// denial describes that source's response, not a local membership change.
func NewSOSync(
	le *logrus.Entry,
	b bus.Bus,
	soID string,
	localObjectPeerID peer.ID,
	localObjectKey crypto.PrivKey,
	soHost *sobject.SOHost,
	peerAdmission func(peer.ID, bool),
	accessValidators ...SnapshotAccessValidator,
) *SOSync {
	// Select the optional inbound-snapshot authority check.
	var validateSnapshotAccess SnapshotAccessValidator
	if len(accessValidators) != 0 {
		validateSnapshotAccess = accessValidators[0]
	}

	// Bind synchronization to the object and its participant identity.
	return &SOSync{
		le:                     le.WithField("so-sync", soID),
		b:                      b,
		soID:                   soID,
		localObjectPeerID:      localObjectPeerID,
		localObjectKey:         localObjectKey,
		soHost:                 soHost,
		validateSnapshotAccess: validateSnapshotAccess,
		peerAdmission:          peerAdmission,
	}
}

// SetPeerRecoveryObserver configures source-recovery observation before Execute
// starts. The observer must return promptly; calls can come from concurrent peer
// streams.
func (s *SOSync) SetPeerRecoveryObserver(observer func(peer.ID, bool)) {
	s.peerRecovery = observer
}

// Execute runs the SO sync, emitting a SolicitProtocol directive and
// handling matched streams until ctx is canceled.
func (s *SOSync) Execute(ctx context.Context) error {
	// Retry only generations ended by recoverable stream failures.
	retryBackoff := newSyncRetryBackoff()
	for {
		retry, err := s.runSolicitationGeneration(ctx)
		if !retry {
			return err
		}

		// Bound repeated failures without abandoning the standing sync demand.
		delay := retryBackoff.NextBackOff()
		if delay == cbackoff.Stop {
			return err
		}
		s.le.WithError(err).WithField("retry-after", delay).
			Debug("rearming shared object synchronization")
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
}

// newSyncRetryBackoff constructs the capped delay between solicitation
// generations.
func newSyncRetryBackoff() cbackoff.BackOff {
	return cbackoff.NewExponentialBackOff(
		cbackoff.WithInitialInterval(syncRetryInitialInterval),
		cbackoff.WithMultiplier(syncRetryMultiplier),
		cbackoff.WithMaxInterval(syncRetryMaxInterval),
		cbackoff.WithRandomizationFactor(0),
		cbackoff.WithMaxElapsedTime(0),
	)
}

// runSolicitationGeneration admits peer streams until cancellation or the
// first recoverable stream failure. retry is true only when Execute should
// create a fresh directive incarnation after backoff.
func (s *SOSync) runSolicitationGeneration(ctx context.Context) (bool, error) {
	// Give every accepted stream the generation's cancellation boundary.
	generationCtx, cancel := context.WithCancel(ctx)
	generation := &solicitationGeneration{}

	// Publish one solicitation lifetime and classify each accepted stream result.
	dir := link_solicit.NewSolicitProtocol(SyncProtocolID, []byte(s.soID), "", 0)
	_, solicitRef, err := s.b.AddDirective(
		dir,
		directive.NewTypedCallbackHandler[link_solicit.SolicitMountedStream](
			func(v directive.TypedAttachedValue[link_solicit.SolicitMountedStream]) {
				generation.startWorker(func() error {
					// Run the accepted stream inside this generation's lifetime.
					err := s.handleSolicitedStream(generationCtx, v.GetValue())

					// Keep typed peer states on the standing directive; return only
					// recoverable failures to the generation owner.
					if err == nil || generationCtx.Err() != nil || isTerminalSyncError(err) {
						return nil
					}
					return err
				})
			},
			nil, nil, nil,
		),
	)
	if err != nil {
		generation.stopAndWait(cancel)
		return false, err
	}
	defer solicitRef.Release()
	defer generation.stopAndWait(cancel)

	// Keep the directive admitted through terminal peer states; only a
	// recoverable stream error retires this generation.
	err = generation.wait(generationCtx)
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	return true, err
}

// isTerminalSyncError reports peer states that require authority or recovery
// to change before another stream can make progress.
func isTerminalSyncError(err error) bool {
	return errors.Is(err, ErrAccessDenied) ||
		errors.Is(err, sobject.ErrParticipantRevoked) ||
		errors.Is(err, sobject.ErrConfigHistoryUnavailable) ||
		errors.Is(err, sobject.ErrStateTooLarge)
}

// syncFailures remembers the last stream failure logged for each peer, so a
// failure that repeats on every retry is logged once.
type syncFailures struct {
	// mtx guards last.
	mtx sync.Mutex
	// last maps a peer to the text of its last logged failure.
	last map[peer.ID]string
}

// record notes the outcome of a stream with remoteID and reports whether it
// changed the peer's logged state. A nil err clears the failure.
func (f *syncFailures) record(remoteID peer.ID, err error) bool {
	// Compare with the peer's last failure.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	prev, failing := f.last[remoteID]
	if err == nil {
		delete(f.last, remoteID)
		return failing
	}

	// Remember the new failure.
	if f.last == nil {
		f.last = make(map[peer.ID]string)
	}
	f.last[remoteID] = err.Error()
	return !failing || prev != err.Error()
}

// startWorker admits one worker before starting it so shutdown cannot miss it.
func (g *solicitationGeneration) startWorker(run func() error) bool {
	// Register the worker only while the generation accepts new streams.
	var started bool
	g.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if g.stopping {
			return
		}
		g.workers++
		started = true
		bcast()
	})
	if !started {
		return false
	}

	// Publish its result and release its generation membership on return.
	go func() {
		g.workerDone(run())
	}()
	return true
}

// workerDone releases one worker and publishes its recoverable failure.
func (g *solicitationGeneration) workerDone(err error) {
	g.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		g.workers--
		if err != nil && g.retryErr == nil && !g.stopping {
			g.retryErr = err
		}
		bcast()
	})
}

// wait blocks until the generation needs retry or its context ends.
func (g *solicitationGeneration) wait(ctx context.Context) error {
	for {
		// Read the result and its matching wait channel atomically.
		var retryErr error
		var waitCh <-chan struct{}
		g.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			retryErr = g.retryErr
			waitCh = getWaitCh()
		})
		if err := ctx.Err(); err != nil {
			return err
		}
		if retryErr != nil {
			return retryErr
		}

		// Wake for either cancellation or a completed worker.
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// stopAndWait fences new workers, cancels accepted streams, and joins them.
func (g *solicitationGeneration) stopAndWait(cancel context.CancelFunc) {
	// Fence callbacks before canceling the shared worker context.
	g.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		g.stopping = true
		bcast()
	})
	cancel()

	// Wait on the same state owner until every accepted worker has returned.
	for {
		var waitCh <-chan struct{}
		var stopped bool
		g.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			stopped = g.workers == 0
			waitCh = getWaitCh()
		})
		if stopped {
			return
		}
		<-waitCh
	}
}

// handleSolicitedStream processes a matched solicit stream for SO sync.
func (s *SOSync) handleSolicitedStream(
	ctx context.Context,
	sms link_solicit.SolicitMountedStream,
) error {
	// Claim the matched stream once for this synchronization worker.
	ms, taken, err := sms.AcceptMountedStream()
	if err != nil {
		return err
	}
	if taken {
		return nil
	}

	// Run the authenticated stream with remote-scoped diagnostics.
	le := s.le.WithField("remote-peer", ms.GetPeerID().String())
	err = s.runStream(
		ctx,
		le,
		ms.GetStream(),
		ms.GetLink().GetLocalPeer(),
		ms.GetPeerID(),
	)
	if ctx.Err() == nil {
		s.logStreamResult(le, ms.GetPeerID(), err)
	}
	return err
}

// logStreamResult logs the end of a stream with remoteID. A change in the
// peer's failure reaches the info or warn log once; a failure that repeats on
// every retry stays at debug.
func (s *SOSync) logStreamResult(le *logrus.Entry, remoteID peer.ID, err error) {
	if !s.failures.record(remoteID, err) {
		if err != nil {
			le.WithError(err).Debug("shared object synchronization ended")
		}
		return
	}
	switch {
	case err == nil:
		le.Info("shared object synchronization recovered")
	case isTerminalSyncError(err):
		le.WithError(err).Warn("shared object synchronization is blocked")
	default:
		le.WithError(err).Info("shared object synchronization ended")
	}
}

// runStream owns authentication, authorization watches, data exchange and stream cleanup.
func (s *SOSync) runStream(
	ctx context.Context,
	le *logrus.Entry,
	strm stream.Stream,
	localTransport peer.ID,
	remoteTransport peer.ID,
) (rerr error) {
	// Preserve the local authorization failure when closing transport interrupts I/O.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	defer strm.Close()
	stopClose := context.AfterFunc(ctx, func() { strm.Close() })
	defer stopClose()

	// Authentication has a short deadline and a smaller frame limit than object data.
	deadline := time.Now().Add(30 * time.Second)
	if contextDeadline, ok := ctx.Deadline(); ok && contextDeadline.Before(deadline) {
		deadline = contextDeadline
	}
	if err := strm.SetDeadline(deadline); err != nil {
		return err
	}
	remoteID, err := s.authenticate(ctx, stream_packet.NewSession(strm, 64*1024), localTransport, remoteTransport)
	defer func() {
		if remoteID != "" && errors.Is(rerr, sobject.ErrConfigHistoryUnavailable) && s.peerRecovery != nil {
			s.peerRecovery(remoteID, true)
		}
	}()
	if err != nil {
		return err
	}
	if err := strm.SetDeadline(time.Time{}); err != nil {
		return err
	}

	// A separate watch bounds revocation even while another sender is blocked.
	sess := stream_packet.NewSession(strm, maxMessageSize)
	watcher := routine.NewRoutineContainer()
	watcher.SetRoutine(func(ctx context.Context) error {
		return s.watchAuthority(ctx, strm, sess, remoteID, cancel)
	})
	watcher.SetContext(ctx, false)
	defer func() {
		strm.Close()
		joinSyncWorkers(watcher)
		if cause := context.Cause(ctx); errors.Is(cause, ErrAccessDenied) {
			rerr = cause
		}
	}()

	return s.synchronize(ctx, le, sess, remoteID)
}

// joinSyncWorkers removes each routine and waits for its body to return.
// The caller cancels and closes transport first to release blocked worker I/O.
func joinSyncWorkers(workers ...*routine.RoutineContainer) {
	for _, worker := range workers {
		if exited, _ := worker.SetRoutine(nil); exited != nil {
			<-exited
		}
	}
}

// watchAuthority retains current authority until revocation, provider failure or cancellation.
// Every exit releases the watch before canceling the owner and closing blocked transport.
func (s *SOSync) watchAuthority(ctx context.Context, strm stream.Stream, sess *stream_packet.Session, remoteID peer.ID, cancel context.CancelCauseFunc) (rerr error) {

	// func.
	defer func() {
		cancel(rerr)
		strm.Close()
	}()
	states, release, err := s.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer release()
	var previous *sobject.SOState
	for {
		current, err := states.WaitValueChange(ctx, previous, nil)
		if err != nil {
			return err
		}
		if err := s.authorizeParticipants(current, remoteID); err != nil {
			// A control frame exposes no state; a stalled transport still closes promptly.
			if strm.SetWriteDeadline(time.Now().Add(250*time.Millisecond)) == nil {
				_ = sendAccessDenied(sess)
			}
			return err
		}
		previous = current
	}
}

// sendAccessDenied reports revocation without sending object state or history.
// The stream's authority watch bounds this write before closing the transport.
func sendAccessDenied(sess *stream_packet.Session) error {
	return sess.SendMsg(&SOSyncMessage{Body: &SOSyncMessage_Authorization{Authorization: &SOSyncAuthorization{}}})
}
