package sobject_sync

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
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
	// carries one whole state, and a history page fills up to the same bound
	// so a single large record, such as a merge, still fits.
	maxMessageSize = sobject.MaxSyncStateSize
	// authMessageSize bounds each authentication frame, including the config
	// changes offered to a participant that refused admission.
	authMessageSize = 64 * 1024
	// syncRetryInitialInterval is a peer's first delay before renewing its
	// offer after its stream fails recoverably.
	syncRetryInitialInterval = 250 * time.Millisecond
	// syncRetryMaxInterval caps a peer's repeated renewal delays.
	syncRetryMaxInterval = 2 * time.Second
	// syncRetryMultiplier doubles the delay until syncRetryMaxInterval.
	syncRetryMultiplier = 2
	// syncRenewGrace delays the renewal on the side with the higher transport
	// peer ID, so the lower side's renewal normally reopens the stream alone.
	syncRenewGrace = 2 * time.Second
	// syncRenewMaxInterval caps the delay between repeated renewals of a
	// peer's offer, which repeat because a failed stream open is not retried.
	syncRenewMaxInterval = 30 * time.Second
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
	// observer receives the per-peer events; fixed at construction.
	observer PeerObserver
	// failures remembers the last logged stream failure of each peer.
	failures syncFailures
	// digest memoizes the digest of the latest watched state for every stream.
	digest stateDigest
}

// PeerObserver receives the per-peer events of one SOSync.
//
// NewSOSync takes the observer, so it is fixed for the syncer's lifetime.
// Calls can come from concurrent peer streams, so every method must be safe
// for concurrent use and return promptly.
type PeerObserver interface {
	// PeerAdmission reports an explicit admission response from a peer that
	// local authority permits. A denial describes that source's response, not
	// a local membership change.
	PeerAdmission(remoteID peer.ID, accepted bool)
	// PeerRecovery reports whether a trusted peer requires source recovery,
	// independently of admission.
	PeerRecovery(remoteID peer.ID, required bool)
	// PeerConnection reports connected once a participant authenticates, then
	// not connected with the error that ended the stream.
	PeerConnection(remoteID peer.ID, connected bool, err error)
	// PeerState reports the StateHash digest of each state the peer advertises
	// as its own. A digest is the peer's claim, not authority: use it only to
	// delay local work, never to admit anything. stateHash is owned by the
	// observer.
	PeerState(remoteID peer.ID, stateHash []byte)
}

// HealthObserver records admission, recovery and connection events in a
// SharedObjectHealth snapshot through the swap function the observer is.
// The swap must apply fn atomically to the current snapshot. PeerState is
// ignored; wrap the observer to handle it.
type HealthObserver func(fn func(*sobject.SharedObjectHealth) *sobject.SharedObjectHealth)

// PeerAdmission records the peer's admission response.
func (o HealthObserver) PeerAdmission(remoteID peer.ID, accepted bool) {
	o(func(h *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
		return h.WithSyncPeerAdmission(remoteID.String(), accepted)
	})
}

// PeerRecovery records whether the peer requires source recovery.
func (o HealthObserver) PeerRecovery(remoteID peer.ID, required bool) {
	o(func(h *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
		return h.WithSyncPeerRecovery(remoteID.String(), required)
	})
}

// PeerConnection records the stream transition at the current time.
func (o HealthObserver) PeerConnection(remoteID peer.ID, connected bool, err error) {
	o(func(h *sobject.SharedObjectHealth) *sobject.SharedObjectHealth {
		return h.WithSyncPeerConnection(remoteID.String(), connected, err, time.Now())
	})
}

// PeerState ignores the advertised digest.
func (HealthObserver) PeerState(peer.ID, []byte) {}

// WithPeerState returns observer with its PeerState events sent to fn instead.
func WithPeerState(observer PeerObserver, fn func(remoteID peer.ID, stateHash []byte)) PeerObserver {
	return peerStateObserver{PeerObserver: observer, state: fn}
}

// peerStateObserver overrides the PeerState events of an observer.
type peerStateObserver struct {
	PeerObserver
	// state receives each advertised digest.
	state func(peer.ID, []byte)
}

// PeerState sends the advertised digest to state.
func (o peerStateObserver) PeerState(remoteID peer.ID, stateHash []byte) {
	o.state(remoteID, stateHash)
}

// NewSOSync constructs a new SOSync.
//
// localObjectPeerID is the local storage identity checked against inbound
// state. The transport peer routes the sync stream but need not be a Space
// participant. localObjectKey must belong to localObjectPeerID and remains
// available for the SOSync lifetime. Authentication rejects a mismatched key.
// observer receives the per-peer events for the SOSync lifetime and must not be
// nil.
func NewSOSync(
	le *logrus.Entry,
	b bus.Bus,
	soID string,
	localObjectPeerID peer.ID,
	localObjectKey crypto.PrivKey,
	soHost *sobject.SOHost,
	observer PeerObserver,
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
		observer:               observer,
	}
}

// Execute runs the SO sync until ctx is canceled. It offers the protocol for
// the object to every peer for its whole lifetime and runs each matched
// stream; a peer whose stream ends renews only its own offer, so the other
// peers' streams keep running.
func (s *SOSync) Execute(ctx context.Context) error {
	// Give every stream and renewal the owner's cancellation boundary.
	streamsCtx, cancel := context.WithCancel(ctx)
	streams := newPeerStreams(streamsCtx, s)

	// Keep the standing offer to every peer for the whole lifetime.
	ref, err := s.addSolicitation(streamsCtx, streams, "")
	if err != nil {
		streams.stop(cancel)
		return err
	}

	// Withdraw the offer before joining, so unclaimed streams close instead of
	// waiting on the join.
	<-ctx.Done()
	ref.Release()
	streams.stop(cancel)
	return ctx.Err()
}

// addSolicitation offers the protocol for the object and hands each matched
// stream to streams. An empty remote makes the standing offer to every peer.
// Otherwise the offer is a new incarnation scoped to remote, which pairs anew
// with the remote's standing offer and so opens a new stream on a link where
// the standing offers already paired.
func (s *SOSync) addSolicitation(ctx context.Context, streams *peerStreams, remote peer.ID) (directive.Reference, error) {
	// Select the standing or the renewed offer.
	dir := link_solicit.NewSolicitProtocol(SyncProtocolID, []byte(s.soID), "", 0)
	if remote != "" {
		dir = link_solicit.NewUniqueSolicitProtocol(SyncProtocolID, []byte(s.soID), remote, 0)
	}

	// Run every stream the offer matches under the stream owner.
	_, ref, err := s.b.AddDirective(
		ctx,
		dir,
		directive.NewTypedCallbackHandler[link_solicit.SolicitMountedStream](
			func(v directive.TypedAttachedValue[link_solicit.SolicitMountedStream]) {
				streams.startStream(v.GetValue())
			},
			nil, nil, nil,
		),
	)
	return ref, err
}

// newSyncRetryBackoff constructs the capped delay before a peer renews its
// offer after its stream fails recoverably.
func newSyncRetryBackoff() cbackoff.BackOff {
	return newCappedBackoff(syncRetryInitialInterval, syncRetryMaxInterval)
}

// newSyncRenewBackoff constructs the capped delay between repeated renewals of
// one peer's offer while no stream with the peer opens.
func newSyncRenewBackoff() cbackoff.BackOff {
	return newCappedBackoff(syncRenewGrace, syncRenewMaxInterval)
}

// newCappedBackoff constructs a doubling delay from initial up to ceiling that
// never gives up.
func newCappedBackoff(initial, ceiling time.Duration) cbackoff.BackOff {
	return cbackoff.NewExponentialBackOff(
		cbackoff.WithInitialInterval(initial),
		cbackoff.WithMultiplier(syncRetryMultiplier),
		cbackoff.WithMaxInterval(ceiling),
		cbackoff.WithRandomizationFactor(0),
		cbackoff.WithMaxElapsedTime(0),
	)
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

// localRefusal is ErrAccessDenied from this side's held authority, naming the
// participant it refused. A denial sent by the remote side is plain
// ErrAccessDenied, because only the remote config can lift it.
type localRefusal struct {
	// remoteID is the refused participant.
	remoteID peer.ID
}

// Error returns the ErrAccessDenied text.
func (r *localRefusal) Error() string {
	return ErrAccessDenied.Error()
}

// Unwrap matches ErrAccessDenied.
func (r *localRefusal) Unwrap() error {
	return ErrAccessDenied
}

// waitReadmission watches the held state until it grants remoteID readable
// access, then returns nil. It returns the context error on cancellation.
// Each state change costs one scan of the participant list.
func (s *SOSync) waitReadmission(ctx context.Context, remoteID peer.ID) error {
	// Watch the same held state that admission reads.
	states, release, err := s.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer release()

	// Start from the current state: the change may already be held.
	var previous *sobject.SOState
	for {
		current, err := states.WaitValueChange(ctx, previous, nil)
		if err != nil {
			return err
		}
		if !errors.Is(s.authorizeParticipants(current, remoteID), ErrAccessDenied) {
			return nil
		}
		previous = current
	}
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

// runStream owns authentication, authorization watches, data exchange and
// stream cleanup. A denial by this side's held authority, at admission or by
// revocation, returns a *localRefusal naming the participant.
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
	remoteID, err := s.authenticate(ctx, stream_packet.NewSession(strm, authMessageSize), localTransport, remoteTransport)
	defer func() {
		if remoteID != "" && errors.Is(rerr, sobject.ErrConfigHistoryUnavailable) {
			s.observer.PeerRecovery(remoteID, true)
		}
	}()
	if err != nil {
		// Only local admission returns the remote identity with an error.
		if remoteID != "" && errors.Is(err, ErrAccessDenied) {
			return &localRefusal{remoteID: remoteID}
		}
		return err
	}

	// Report the authenticated stream until it ends.
	s.observer.PeerConnection(remoteID, true, nil)
	defer func() { s.observer.PeerConnection(remoteID, false, rerr) }()
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
			rerr = &localRefusal{remoteID: remoteID}
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

// _ is a type assertion.
var (
	_ PeerObserver = HealthObserver(nil)
	_ PeerObserver = peerStateObserver{}
)
