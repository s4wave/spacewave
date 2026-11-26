package sobject_sync

import (
	"bytes"
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

// PeerLinks reports which transport peers the session holds links to, with
// channels that close when that set may have changed.
// *transport.SessionTransport implements it.
type PeerLinks interface {
	// GetLinkedPeerIDsSnapshotWithWait returns the peers of peerIDs that have a
	// link, and channels that close when the link set changes.
	GetLinkedPeerIDsSnapshotWithWait(peerIDs []peer.ID) (map[peer.ID]struct{}, []<-chan struct{})
}

// SOSync synchronizes one shared object over the session transport's child bus.
type SOSync struct {
	// le records synchronization errors and peer activity.
	le *logrus.Entry
	// b supplies the session transport's solicitation interface.
	b bus.Bus
	// links reports which transport peers the session holds links to.
	links PeerLinks
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
// nil. links reports the session's links, and bounds each peer's renewal to
// the time a link to the peer exists; Execute requires it.
func NewSOSync(
	le *logrus.Entry,
	b bus.Bus,
	links PeerLinks,
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
		links:                  links,
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

// waitHeld watches the held state until ready reports true for it, then
// returns nil. It returns the context error on cancellation. ready runs once
// for each held state, starting with the current one.
func (s *SOSync) waitHeld(ctx context.Context, ready func(*sobject.SOState) bool) error {
	// Watch the same held state that admission and catch-up read.
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
		if ready(current) {
			return nil
		}
		previous = current
	}
}

// endStep is what lets a peer stream again after its last stream ended.
type endStep int

const (
	// stepPark leaves the peer to the remote side and to a new link.
	stepPark endStep = iota
	// stepRetry backs off and renews, since a fresh stream may clear the end.
	stepRetry
	// stepWatch renews at once when a held state lifts the end.
	stepWatch
)

// nextStep classifies a stream that returned err by the event that can make
// the peer syncable again. For stepWatch, ready reports the held state that
// lifts the end. mark is the held authority the stream ran against, so a
// change that landed before the watch starts still lifts the end.
func (s *SOSync) nextStep(err error, mark heldMark) (step endStep, ready func(*sobject.SOState) bool) {
	var refused *localRefusal
	switch {
	case errors.As(err, &refused):
		// Held authority refused the participant: a config that readmits it
		// lifts the refusal.
		return stepWatch, func(state *sobject.SOState) bool {
			return !errors.Is(s.authorizeParticipants(state, refused.remoteID), ErrAccessDenied)
		}
	case errors.Is(err, sobject.ErrParticipantRevoked):
		// A committed removal of this participant: a config that grants it
		// readable access again lifts it.
		return stepWatch, s.localReadable
	case errors.Is(err, sobject.ErrConfigHistoryUnavailable), errors.Is(err, sobject.ErrStateTooLarge):
		// Catch-up cannot bridge the two states: a recovered or advanced
		// config, or a checkpoint that folds the operations above it, may.
		return stepWatch, mark.changed
	case errors.Is(err, ErrAccessDenied):
		// The remote's held authority refused this side. Only the remote's
		// config lifts it, and the remote's own refusal watch renews.
		return stepPark, nil
	default:
		return stepRetry, nil
	}
}

// localReadable reports whether state grants the local participant readable
// access.
func (s *SOSync) localReadable(state *sobject.SOState) bool {
	for _, participant := range state.GetConfig().GetParticipants() {
		if participant.GetPeerId() == s.localObjectPeerID.String() && sobject.CanReadState(participant.GetRole()) {
			return true
		}
	}
	return false
}

// heldMark is the held authority one stream ran against: the config head and
// the checkpoint, which bound what catch-up can bridge.
type heldMark struct {
	// configChainHash is the held config head.
	configChainHash []byte
	// checkpoint is the held checkpoint's inner bytes.
	checkpoint []byte
}

// readHeldMark marks the current held state. A stream reads it before any
// other held state, so every state the stream compares is at or after it.
func (s *SOSync) readHeldMark(ctx context.Context) (heldMark, error) {
	// Read the current held state through the host watch.
	states, release, err := s.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return heldMark{}, err
	}
	defer release()
	state, err := states.WaitValue(ctx, nil)
	if err != nil {
		return heldMark{}, err
	}

	// Keep only the two fields that decide catch-up; held states are immutable.
	return heldMark{
		configChainHash: state.GetConfig().GetConfigChainHash(),
		checkpoint:      state.GetCheckpoint().GetInner(),
	}, nil
}

// changed reports whether state holds a different config head or checkpoint
// than the mark. Operations above the checkpoint do not count, so a busy object
// does not reopen a stream that cannot catch up.
func (m heldMark) changed(state *sobject.SOState) bool {
	return !bytes.Equal(state.GetConfig().GetConfigChainHash(), m.configChainHash) ||
		!bytes.Equal(state.GetCheckpoint().GetInner(), m.checkpoint)
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

// peerStreams owns the streams and renewals of one Execute call, keyed by the
// transport peer each stream runs with. PeerRetry.tla models its lifecycle.
//
// The solicit controller opens one stream for each pair of offers on a link,
// so a stream that ends leaves its link without another until one side offers
// the protocol again. A peer whose last stream ends recoverably renews only
// its own offer: a unique offer scoped to that peer, which pairs with the
// remote's standing offer and opens one new stream on that peer's links. The
// other peers' streams keep running. An end that only a held-state event can
// lift, such as a refusal, a revocation or a history catch-up cannot bridge,
// waits for that event and then renews at once; a denial by the remote side
// renews nothing and leaves the peer to the remote side's renewal.
//
// A renewal lasts only while a link to its peer exists. A new link pairs both
// sides' standing offers and opens a stream without one, so a peer whose last
// link goes costs nothing until it returns.
type peerStreams struct {
	// s runs each stream and supplies held authority for readmission.
	s *SOSync
	// ctx is the owner's lifetime, shared by every stream and renewal.
	ctx context.Context
	// bcast guards stopping, workers and peers.
	bcast broadcast.Broadcast
	// stopping fences new streams and renewals while the owner drains.
	stopping bool
	// workers counts running stream and renewal goroutines.
	workers int
	// peers holds the lifecycle of each transport peer seen so far.
	peers map[peer.ID]*peerStream
}

// peerStream is the lifecycle of one transport peer.
type peerStream struct {
	// streams counts the running streams with the peer, one per link pair.
	streams int
	// renewal reopens the peer's stream; set only while streams is zero and a
	// link to the peer exists.
	renewal *peerRenewal
	// backoff spaces the peer's renewals, from syncRetryInitialInterval up to
	// syncRetryMaxInterval, for the owner's lifetime.
	backoff cbackoff.BackOff
}

// peerRenewal is one running renewal of a peer's offer.
type peerRenewal struct {
	// cancel stops the renewal and link watch goroutines.
	cancel context.CancelFunc
	// ref is the latest renewed offer, nil before the first. Whoever removes
	// the renewal from its peerStream releases it.
	ref directive.Reference
}

// newPeerStreams constructs the stream owner for one Execute lifetime.
func newPeerStreams(ctx context.Context, s *SOSync) *peerStreams {
	return &peerStreams{s: s, ctx: ctx, peers: make(map[peer.ID]*peerStream)}
}

// startStream runs a matched stream in its own goroutine, registered before it
// starts so shutdown cannot miss it. A stream matched after the owner began
// stopping is left unclaimed, and the solicit controller closes it.
func (o *peerStreams) startStream(sms link_solicit.SolicitMountedStream) {
	if o.addWorker() {
		go o.runStream(sms)
	}
}

// runStream claims a matched stream, runs it, and decides how the peer
// continues once it ends.
func (o *peerStreams) runStream(sms link_solicit.SolicitMountedStream) {
	// Release the goroutine's registration however the stream ends.
	defer o.workerDone()

	// Claim the stream once; a failed claim names no peer to renew.
	ms, taken, err := sms.AcceptMountedStream()
	if err != nil {
		o.s.le.WithError(err).Debug("matched shared object stream was not claimed")
		return
	}
	if taken {
		return
	}

	// Count the stream against its peer, ending any renewal for it.
	remote := ms.GetPeerID()
	local := ms.GetLink().GetLocalPeer()
	o.arrive(remote)

	// Mark the held authority before the stream reads any held state, so a
	// held-state watch after it ends misses no change.
	le := o.s.le.WithField("remote-peer", remote.String())
	mark, err := o.s.readHeldMark(o.ctx)
	if err != nil {
		ms.GetStream().Close()
		o.end(remote, local < remote, err, mark)
		return
	}

	// Run the authenticated stream with remote-scoped diagnostics.
	err = o.s.runStream(o.ctx, le, ms.GetStream(), local, remote)
	if o.ctx.Err() == nil {
		o.s.logStreamResult(le, remote, err)
	}
	o.end(remote, local < remote, err, mark)
}

// arrive counts a new stream with remote and removes the peer's renewal, which
// a stream from any offer makes unnecessary. Releasing the renewed offer
// refuses its other pairs; the accepted stream keeps running.
func (o *peerStreams) arrive(remote peer.ID) {
	// Take the renewal while counting the stream.
	var renewal *peerRenewal
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		p := o.peerLocked(remote)
		p.streams++
		renewal, p.renewal = p.renewal, nil
		bcast()
	})

	// Stop the renewal and withdraw its offer.
	if renewal != nil {
		renewal.stop()
	}
}

// end uncounts a stream with remote that returned err, and starts the peer's
// renewal when this was its last stream and err leaves a next step. lower
// reports whether the local transport peer ID sorts below remote's. mark is
// the held authority the stream ran against.
func (o *peerStreams) end(remote peer.ID, lower bool, err error, mark heldMark) {
	// Classify the end by the event that lets the peer stream again.
	step, ready := o.s.nextStep(err, mark)

	// Uncount the stream, and register the renewal only for the last one.
	var renewal *peerRenewal
	var wait time.Duration
	var rctx context.Context
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		// Uncount the stream; renew only after the peer's last one.
		p := o.peerLocked(remote)
		p.streams--
		bcast()
		if step == stepPark || p.streams != 0 || o.stopping || o.ctx.Err() != nil {
			return
		}

		// Space recoverable retries. The side with the higher peer ID also
		// waits out the grace, so the lower side's renewal normally arrives
		// first and the two sides do not open a stream each.
		if step == stepRetry {
			wait = p.backoff.NextBackOff()
			if !lower {
				wait += syncRenewGrace
			}
		}
		var cancel context.CancelFunc
		rctx, cancel = context.WithCancel(o.ctx)
		renewal = &peerRenewal{cancel: cancel}
		p.renewal = renewal
		o.workers += 2
	})
	if renewal == nil {
		return
	}

	// Retire the renewal once the peer's last link goes.
	go o.watchLink(rctx, remote, renewal)

	// Wait for the lifting event, then renew until a stream with the peer
	// arrives, its last link goes, or the owner stops.
	go func() {
		defer o.workerDone()
		if ready != nil {
			if err := o.s.waitHeld(rctx, ready); err != nil {
				// Without the held state this side cannot sync either; park
				// the peer until a new link or the next Execute.
				if rctx.Err() == nil {
					o.s.le.WithError(err).WithField("remote-peer", remote.String()).
						Warn("failed to watch held state for the peer's next stream")
					o.retire(remote, renewal)
				}
				return
			}
		}
		o.renew(rctx, remote, renewal, wait)
	}()
}

// watchLink retires renewal once the session holds no link to remote, and
// returns when the renewal ends. Each change to the session's link set costs
// one lookup per running renewal.
func (o *peerStreams) watchLink(rctx context.Context, remote peer.ID, renewal *peerRenewal) {
	defer o.workerDone()
	peers := []peer.ID{remote}
	for {
		// Retire the renewal when the peer has no link left.
		linked, changed := o.s.links.GetLinkedPeerIDsSnapshotWithWait(peers)
		if _, ok := linked[remote]; !ok {
			o.retire(remote, renewal)
			return
		}

		// Wait for the next change to the link set.
		if err := broadcast.WaitAny(rctx, changed...); err != nil {
			return
		}
	}
}

// retire removes renewal from remote's lifecycle while it is still the
// peer's, then stops it and withdraws its offer. An arriving stream or
// shutdown that took it first has already stopped it.
func (o *peerStreams) retire(remote peer.ID, renewal *peerRenewal) {
	// Take the renewal only while it is still the peer's.
	var current bool
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		p := o.peers[remote]
		current = !o.stopping && p.renewal == renewal
		if current {
			p.renewal = nil
			bcast()
		}
	})

	// Stop the renewal outside the lock.
	if current {
		renewal.stop()
	}
}

// renew waits for wait, then offers the protocol to remote under a new
// incarnation, and repeats with a slower backoff until rctx ends, since the
// solicit controller does not retry a failed open. rctx ends when a stream
// with the peer arrives, its last link goes, or the owner stops.
func (o *peerStreams) renew(rctx context.Context, remote peer.ID, renewal *peerRenewal, wait time.Duration) {
	le := o.s.le.WithField("remote-peer", remote.String())
	repeat := newSyncRenewBackoff()
	for {
		// Wait out the delay unless a stream arrives or the owner stops.
		if wait > 0 {
			le.WithField("retry-after", wait).Debug("renewing shared object synchronization with peer")
			select {
			case <-rctx.Done():
				return
			//gostyle:timer backoff
			case <-time.After(wait):
			}
		}

		// Offer the protocol to the peer and keep the newest offer.
		ref, err := o.s.addSolicitation(o.ctx, o, remote)
		if err != nil {
			le.WithError(err).Warn("failed to renew shared object synchronization with peer")
		} else if !o.replaceRenewal(remote, renewal, ref) {
			return
		}

		// Space the next renewal until a stream with the peer arrives.
		wait = repeat.NextBackOff()
	}
}

// replaceRenewal stores ref as the renewal's newest offer and releases the one
// it replaces. When the renewal was removed, by an arriving stream or by
// shutdown, it releases ref instead and returns false.
func (o *peerStreams) replaceRenewal(remote peer.ID, renewal *peerRenewal, ref directive.Reference) bool {
	// Swap the offer only while the renewal is still the peer's.
	var prev directive.Reference
	var current bool
	o.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = !o.stopping && o.peers[remote].renewal == renewal
		if current {
			prev, renewal.ref = renewal.ref, ref
		}
	})

	// Release the offer that is no longer kept.
	if !current {
		ref.Release()
	} else if prev != nil {
		prev.Release()
	}
	return current
}

// stop fences new streams and renewals, cancels every stream and renewal
// through cancel, withdraws the renewed offers, and waits until every stream
// and renewal goroutine has returned. The caller releases the standing offer
// first, so unclaimed streams close instead of waiting on the join.
func (o *peerStreams) stop(cancel context.CancelFunc) {
	// Fence and take every renewal before canceling the shared context.
	var renewals []*peerRenewal
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		o.stopping = true
		for _, p := range o.peers {
			if p.renewal != nil {
				renewals = append(renewals, p.renewal)
				p.renewal = nil
			}
		}
		bcast()
	})
	cancel()
	for _, renewal := range renewals {
		renewal.stop()
	}

	// Wait on the same state owner until every goroutine has returned.
	for {
		var waitCh <-chan struct{}
		var stopped bool
		o.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			stopped = o.workers == 0
			waitCh = getWaitCh()
		})
		if stopped {
			return
		}
		<-waitCh
	}
}

// addWorker registers a stream goroutine unless the owner is stopping.
func (o *peerStreams) addWorker() bool {
	var added bool
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if o.stopping {
			return
		}
		o.workers++
		added = true
		bcast()
	})
	return added
}

// workerDone releases one stream or renewal goroutine.
func (o *peerStreams) workerDone() {
	o.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		o.workers--
		bcast()
	})
}

// peerLocked returns the lifecycle of remote, creating it on first use.
// Caller must hold bcast.
func (o *peerStreams) peerLocked(remote peer.ID) *peerStream {
	p := o.peers[remote]
	if p == nil {
		p = &peerStream{backoff: newSyncRetryBackoff()}
		o.peers[remote] = p
	}
	return p
}

// stop cancels the renewal goroutine and withdraws its newest offer. The
// caller has removed the renewal from its peerStream, so no new offer is kept.
func (r *peerRenewal) stop() {
	r.cancel()
	if r.ref != nil {
		r.ref.Release()
	}
}

// _ is a type assertion.
var (
	_ PeerObserver = HealthObserver(nil)
	_ PeerObserver = peerStateObserver{}
)
