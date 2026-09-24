package sobject_sync

import (
	"context"
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
	// maxMessageSize is the max message size for SO sync messages.
	maxMessageSize = 10 * 1024 * 1024
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
		errors.Is(err, sobject.ErrConfigHistoryUnavailable)
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
	if err != nil && ctx.Err() == nil {
		le.WithError(err).Debug("shared object synchronization ended")
	}
	return err
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
		if exited, _ := watcher.SetRoutine(nil); exited != nil {
			<-exited
		}
		if cause := context.Cause(ctx); errors.Is(cause, ErrAccessDenied) {
			rerr = cause
		}
	}()

	return s.synchronize(ctx, le, sess, remoteID)
}

// watchAuthority retains current authority until revocation, provider failure or cancellation.
// Every exit releases the watch before canceling the owner and closing blocked transport.
func (s *SOSync) watchAuthority(ctx context.Context, strm stream.Stream, sess *stream_packet.Session, remoteID peer.ID, cancel context.CancelCauseFunc) (rerr error) {
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

// handleRemoteOp processes an operation received from the peer.
func (s *SOSync) handleRemoteOp(ctx context.Context, le *logrus.Entry, syncOp *SOSyncOp) {
	// Ignore frames without a signed operation payload.
	if len(syncOp.GetOperation()) == 0 {
		return
	}

	// Decode the outer signed operation before inspecting its signer.
	op := &sobject.SOOperation{}
	if err := op.UnmarshalVT(syncOp.GetOperation()); err != nil {
		le.WithError(err).Warn("failed to unmarshal remote op")
		return
	}

	// Extract the peer ID from the operation signature to queue it.
	opInner, err := op.UnmarshalInner()
	if err != nil {
		le.WithError(err).Warn("failed to unmarshal remote op inner")
		return
	}

	// Parse the signer's canonical peer identity.
	peerIDStr := opInner.GetPeerId()
	if peerIDStr == "" {
		le.Warn("remote op missing peer id")
		return
	}

	peerID, err := peer.IDB58Decode(peerIDStr)
	if err != nil {
		le.WithError(err).Warn("invalid peer id in remote op")
		return
	}

	// Verify the op signer against the current local participants before
	// queueing. SOState.QueueOperation re-validates under the state lock;
	// this check surfaces rejection of unauthorized ops at warn level.
	localState, err := s.soHost.GetHostState(ctx)
	if err != nil {
		le.WithError(err).Warn("failed to load local state for op authorization")
		return
	}
	if err := op.ValidateSignature(s.soID, localState.GetConfig().GetParticipants()); err != nil {
		le.WithError(err).WithField("op-peer", peerIDStr).
			Warn("rejected unauthorized remote op")
		return
	}
	for _, account := range localState.GetRoot().GetAccountNonces() {
		if account.GetPeerId() == peerIDStr && account.GetNonce() >= opInner.GetNonce() {
			return
		}
	}

	// Ignore an operation whose accepted or rejected identity is already known.
	existing, rejection, err := localState.GetOperationStatus(peerIDStr, opInner.GetLocalId())
	if err != nil {
		le.WithError(err).Debug("failed to inspect remote op status")
		return
	}
	if existing != nil || rejection != nil {
		return
	}

	// Queue the signed operation directly against the SOHost.
	// The SOHost validates signatures and nonces.
	if err := s.soHost.QueueOperation(ctx, peerID, func(nonce uint64) (*sobject.SOOperation, error) {
		return op, nil
	}); err != nil {
		le.WithError(err).Debug("failed to queue remote op")
	}
}
