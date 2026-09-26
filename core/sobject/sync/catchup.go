package sobject_sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// maxHistoryPageBytes bounds each complete history frame, including its envelope.
const maxHistoryPageBytes = 1024 * 1024

// maxHistoryPageEntries bounds verification work in one history page.
const maxHistoryPageEntries = 128

// catchupTimeout bounds a pinned advertisement and its request through acknowledgment.
const catchupTimeout = 60 * time.Second

// denialGrace bounds how long a failed write waits for frames the peer sent before closing.
const denialGrace = 250 * time.Millisecond

// syncReceive retains an untrusted suffix until its complete target snapshot arrives.
type syncReceive struct {
	// head pins the target and its revision for this request.
	head *SOSyncHead
	// base is the checkpoint held when requesting the suffix.
	base []byte
	// cursor is the hash after the last received entry.
	cursor []byte
	// changes remains untrusted until the host verifies the complete suffix.
	changes []*sobject.SOConfigChange
	// size counts serialized retained history bytes.
	size int
	// deadline includes history transfer, snapshot validation and persistence.
	deadline time.Time
}

// syncResponse holds one bounded response while the writer drains its frames.
type syncResponse struct {
	// revision identifies the pinned advertisement.
	revision uint64
	// cursor is the head before the next page.
	cursor []byte
	// changes contains the unsent suffix, in causal order.
	changes []*sobject.SOConfigChange
	// snapshot completes the response after all history pages.
	snapshot *SOSyncSnapshot
}

// syncIncoming is one received frame or the terminal read failure.
type syncIncoming struct {
	// message is nil on read failure.
	message *SOSyncMessage
	// err terminates the exchange when transport stops.
	err error
}

// handleDenial reports a peer's explicit rejection and ends the exchange.
func (s *SOSync) handleDenial(remoteID peer.ID, authorization *SOSyncAuthorization) error {
	if !authorization.GetAccepted() && s.peerAdmission != nil {
		s.peerAdmission(remoteID, false)
	}
	return ErrAccessDenied
}

// drainDenial prefers a denial the peer sent before closing over the write
// failure its close caused. The reader delivers frames in order, so frames
// received before the close precede its terminal error. Returns writeErr when
// no denial arrives within denialGrace.
func (s *SOSync) drainDenial(ctx context.Context, incoming <-chan syncIncoming, remoteID peer.ID, writeErr error) error {
	timer := time.NewTimer(denialGrace)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return writeErr
		case <-timer.C:
			return writeErr
		case received := <-incoming:
			if received.err != nil {
				return writeErr
			}
			if authorization := received.message.GetAuthorization(); authorization != nil {
				return s.handleDenial(remoteID, authorization)
			}
		}
	}
}

// synchronize owns head negotiation, bounded catch-up and continuing state updates.
// One reader and one writer make simultaneous catch-up safe on unbuffered transports.
// Only this loop owns requests and cursors; every worker is joined before return.
func (s *SOSync) synchronize(ctx context.Context, le *logrus.Entry, sess *stream_packet.Session, remoteID peer.ID) error {
	// Retain the actual host watch for the complete stream lifetime.
	states, release, err := s.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithCancel(ctx)
	incoming := make(chan syncIncoming, 1)
	outbound := make(chan *SOSyncMessage)
	sent := make(chan error, 1)
	changed := make(chan struct{}, 1)

	// Bound read-ahead to one frame and keep transport failures observable.
	reader := routine.NewRoutineContainer()
	reader.SetRoutine(func(ctx context.Context) error {
		defer sess.Close()
		for {
			message := &SOSyncMessage{}
			err := sess.RecvMsg(message)
			if errors.Is(err, stream_packet.ErrMessageTooLarge) {
				err = errors.Wrap(sobject.ErrConfigHistoryUnavailable, "peer snapshot exceeds frame budget")
			}
			select {
			case incoming <- syncIncoming{message: message, err: err}:
			case <-ctx.Done():
				return ctx.Err()
			}
			if err != nil {
				return err
			}
		}
	})

	// Recheck current admission immediately before each serialized outbound frame.
	// A failed write leaves the session open so the loop can drain delivered frames.
	writer := routine.NewRoutineContainer()
	writer.SetRoutine(func(ctx context.Context) error {
		return s.writeMessages(ctx, sess, remoteID, states, outbound, sent)
	})

	// Coalesce state changes while a prior advertisement is pinned by its receiver.
	watcher := routine.NewRoutineContainer()
	watcher.SetRoutine(func(ctx context.Context) error {
		var previous *sobject.SOState
		for {
			current, err := states.WaitValueChange(ctx, previous, nil)
			if err != nil {
				return err
			}
			select {
			case changed <- struct{}{}:
			default:
			}
			previous = current
		}
	})
	workers := []*routine.RoutineContainer{reader, writer, watcher}
	for _, worker := range workers {
		worker.SetContext(ctx, false)
	}
	defer func() {
		cancel()
		sess.Close()
		joinSyncWorkers(workers...)
	}()

	// Keep at most one advertisement, response, incoming suffix and control frame.
	x := &syncExchange{sync: s, remoteID: remoteID}
	timer := time.NewTimer(catchupTimeout)
	timer.Stop()
	defer timer.Stop()
	for {
		if err := x.advance(ctx, le, sess, states, incoming, outbound, sent, changed, timer); err != nil {
			return err
		}
	}
}

// syncExchange is the negotiation state owned exclusively by synchronize's loop.
// Worker routines receive frames and results through channels and never mutate this state.
type syncExchange struct {
	// sync supplies held-state verification, history and participant observers.
	sync *SOSync
	// remoteID is the participant established by the stream's mutual authentication.
	remoteID peer.ID
	// advertised pins the local checkpoint until its acknowledgment.
	advertised *sobject.SOState
	// lastAdvertised suppresses repeated advertisement of an unchanged checkpoint.
	lastAdvertised *sobject.SOState
	// revision identifies the current local advertisement without reuse.
	revision uint64
	// remoteRevision rejects repeated or reordered remote advertisements.
	remoteRevision uint64
	// advertisementDeadline bounds the local response through acknowledgment.
	advertisementDeadline time.Time
	// requested admits at most one history request for the pinned advertisement.
	requested bool
	// receiving retains the untrusted remote suffix and its original deadline.
	receiving *syncReceive
	// response retains the pinned local suffix while pages drain.
	response *syncResponse
	// control precedes response data at the next available writer handoff.
	control *SOSyncMessage
	// outgoing is the frame prepared for the next writer handoff.
	outgoing *SOSyncMessage
	// inFlight is the handed-off frame awaiting its writer result.
	inFlight *SOSyncMessage
	// terminal prevents further data admission while recovery notification drains.
	terminal error
}

// advance reads current authority, prepares output and handles one selected channel event.
// Only synchronize calls it in production; its worker channels retain the existing ownership.
func (x *syncExchange) advance(
	ctx context.Context, le *logrus.Entry, sess *stream_packet.Session,
	states ccontainer.Watchable[*sobject.SOState], incoming <-chan syncIncoming,
	outbound chan<- *SOSyncMessage, sent <-chan error, changed <-chan struct{}, timer *time.Timer,
) error {
	// Controls precede response pages; a new head waits for the previous acknowledgment.
	current := states.GetValue()
	if err := x.sync.authorizeParticipants(current, x.remoteID); err != nil {
		_ = sendAccessDenied(sess)
		return err
	}
	if err := x.prepareSend(current); err != nil {
		return err
	}

	// One timer covers both directions without extending the budget on each page.
	deadline := x.advertisementDeadline
	if x.receiving != nil && (deadline.IsZero() || x.receiving.deadline.Before(deadline)) {
		deadline = x.receiving.deadline
	}
	var expired <-chan time.Time
	if !deadline.IsZero() {
		timer.Reset(time.Until(deadline))
		expired = timer.C
	} else {
		timer.Stop()
	}
	var send chan<- *SOSyncMessage
	if x.outgoing != nil && x.inFlight == nil {
		send = outbound
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-expired:
		return context.DeadlineExceeded
	case <-changed:
		// The next iteration reads the latest authoritative state.
	case send <- x.outgoing:
		x.inFlight, x.outgoing = x.outgoing, nil
	case err := <-sent:
		if err != nil {
			return x.sync.drainDenial(ctx, incoming, x.remoteID, err)
		}
		if x.inFlight.GetRecoveryRequired() != nil {
			return sobject.ErrConfigHistoryUnavailable
		}
		x.inFlight = nil
	case received := <-incoming:
		if received.err != nil {
			if x.terminal != nil {
				return x.terminal
			}
			return received.err
		}
		current = states.GetValue()
		if err := x.sync.authorizeParticipants(current, x.remoteID); err != nil {
			_ = sendAccessDenied(sess)
			return err
		}
		if err := x.receive(ctx, le, current, received.message); err != nil {
			return err
		}
	}
	return nil
}

// prepareSend fills the next writer slot without replacing a queued or in-flight frame.
func (x *syncExchange) prepareSend(current *sobject.SOState) error {
	var err error
	if x.outgoing == nil && x.inFlight == nil {
		switch {
		case x.control != nil:
			x.outgoing, x.control = x.control, nil
		case x.response != nil:
			x.outgoing, err = x.response.nextMessage()
			if err != nil {
				return err
			}
			if x.outgoing.GetSnapshot() != nil {
				x.response = nil
			}
		case x.advertised == nil && current != nil && !current.EqualVT(x.lastAdvertised):
			x.revision++
			if x.revision == 0 {
				return errors.New("sync revision exhausted")
			}
			digest, err := syncStateHash(current)
			if err != nil {
				return err
			}
			x.advertised, x.lastAdvertised = current, current
			x.advertisementDeadline = time.Now().Add(catchupTimeout)
			x.requested = false
			x.outgoing = &SOSyncMessage{Body: &SOSyncMessage_Head{Head: &SOSyncHead{
				Revision: x.revision, ConfigHash: bytes.Clone(current.GetConfig().GetConfigChainHash()),
				ConfigSeqno: current.GetConfig().GetConfigChainSeqno(), RootSeqno: current.GetRoot().GetInnerSeqno(), StateHash: digest,
			}}}
		}
	}
	return nil
}

// receive consumes one frame after the owner has checked current participant authority.
// Terminal recovery still observes explicit denials while discarding later data frames.
func (x *syncExchange) receive(ctx context.Context, le *logrus.Entry, current *sobject.SOState, message *SOSyncMessage) error {
	var err error
	if authorization, ok := message.GetBody().(*SOSyncMessage_Authorization); ok {
		return x.sync.handleDenial(x.remoteID, authorization.Authorization)
	}

	// Drain already-sent frames without admitting new data after recovery becomes terminal.
	if x.terminal != nil {
		return nil
	}

	switch body := message.GetBody().(type) {
	case *SOSyncMessage_Head:
		head := body.Head
		if len(head.GetConfigHash()) == 0 {
			return sobject.ErrConfigHistoryUnavailable
		}
		if head.GetRevision() <= x.remoteRevision || len(head.GetConfigHash()) > 128 || len(head.GetStateHash()) != sha256.Size || x.receiving != nil || x.control != nil {
			return errors.New("invalid or overlapping sync head")
		}
		x.remoteRevision = head.GetRevision()
		digest, err := syncStateHash(current)
		if err != nil {
			return err
		}
		if bytes.Equal(head.GetStateHash(), digest) && x.sync.peerRecovery != nil {
			x.sync.peerRecovery(x.remoteID, false)
		}
		config := current.GetConfig()
		if bytes.Equal(head.GetStateHash(), digest) || head.GetConfigSeqno() < config.GetConfigChainSeqno() ||
			(bytes.Equal(head.GetConfigHash(), config.GetConfigChainHash()) && head.GetRootSeqno() < current.GetRoot().GetInnerSeqno()) {
			x.control = syncAcknowledgment(head.GetRevision())
			return nil
		}
		base := bytes.Clone(config.GetConfigChainHash())
		x.receiving = &syncReceive{head: head, base: base, cursor: base, deadline: time.Now().Add(catchupTimeout)}
		x.control = &SOSyncMessage{Body: &SOSyncMessage_HistoryRequest{HistoryRequest: &SOSyncHistoryRequest{
			Revision: head.GetRevision(), BaseHash: base,
		}}}
	case *SOSyncMessage_HistoryRequest:
		request := body.HistoryRequest
		if x.advertised == nil || request.GetRevision() != x.revision || x.requested || len(request.GetBaseHash()) == 0 || len(request.GetBaseHash()) > 128 {
			return errors.New("invalid or repeated sync request")
		}
		x.requested = true
		requestCtx, requestCancel := context.WithDeadline(ctx, x.advertisementDeadline)
		x.response, err = x.sync.prepareResponse(requestCtx, x.advertised, request)
		requestCancel()
		if err != nil {
			x.terminal = sobject.ErrConfigHistoryUnavailable
			x.control = &SOSyncMessage{Body: &SOSyncMessage_RecoveryRequired{RecoveryRequired: &SOSyncRecoveryRequired{Revision: x.revision}}}
		}
	case *SOSyncMessage_HistoryPage:
		if x.receiving == nil {
			return errors.New("unsolicited history page")
		}
		if err := x.receiving.appendPage(message); err != nil {
			if !errors.Is(err, sobject.ErrConfigHistoryUnavailable) {
				return err
			}
			x.terminal = err
			x.control = &SOSyncMessage{Body: &SOSyncMessage_RecoveryRequired{RecoveryRequired: &SOSyncRecoveryRequired{Revision: x.receiving.head.GetRevision()}}}
		}
	case *SOSyncMessage_Snapshot:
		if x.receiving == nil || x.control != nil {
			return errors.Wrap(sobject.ErrConfigHistoryUnavailable, "peer did not use requested snapshot protocol")
		}
		requestCtx, requestCancel := context.WithDeadline(ctx, x.receiving.deadline)
		err := x.sync.acceptResponse(requestCtx, x.receiving, body.Snapshot)
		requestCancel()
		if err != nil {
			return err
		}
		if x.sync.peerRecovery != nil && !x.sync.responseObsolete(ctx, x.receiving.head) {
			x.sync.peerRecovery(x.remoteID, false)
		}
		x.control = syncAcknowledgment(x.receiving.head.GetRevision())
		x.receiving = nil
	case *SOSyncMessage_Ack:
		if x.advertised == nil || body.Ack.GetRevision() != x.revision || x.response != nil {
			return errors.New("invalid sync acknowledgment")
		}
		x.advertised = nil
		x.advertisementDeadline = time.Time{}
	case *SOSyncMessage_RecoveryRequired:
		matchesRequest := x.receiving != nil && body.RecoveryRequired.GetRevision() == x.receiving.head.GetRevision()
		matchesAdvertisement := x.advertised != nil && body.RecoveryRequired.GetRevision() == x.revision
		if !matchesRequest && !matchesAdvertisement {
			return errors.New("unsolicited recovery response")
		}
		return sobject.ErrConfigHistoryUnavailable
	case *SOSyncMessage_Op:
		x.sync.handleRemoteOp(ctx, le, body.Op)
	default:
		return errors.New("unexpected authenticated sync message")
	}
	return nil
}

// writeMessages serializes frames under freshly observed participant authority.
// Each frame waits until the local state it carries is durable.
// The caller owns state retention, outbound/result channels and transport closure.
// Cancellation interrupts channel waits; closing transport releases a blocked write.
// Failures are reported once unless cancellation wins the result-channel wait.
func (s *SOSync) writeMessages(
	ctx context.Context,
	sess *stream_packet.Session,
	remoteID peer.ID,
	states ccontainer.Watchable[*sobject.SOState],
	outbound <-chan *SOSyncMessage,
	sent chan<- error,
) error {
	for {
		var message *SOSyncMessage
		select {
		case message = <-outbound:
		case <-ctx.Done():
			return ctx.Err()
		}
		// The frame captured its state before this wait, so the wait covers it.
		err := s.authorizeParticipants(states.GetValue(), remoteID)
		if err == nil {
			err = s.soHost.WaitDurable(ctx)
			if err == nil {
				err = sess.SendMsg(message)
			}
		} else {
			_ = sendAccessDenied(sess)
		}
		select {
		case sent <- err:
		case <-ctx.Done():
			return ctx.Err()
		}
		if err != nil {
			return err
		}
	}
}

// syncAcknowledgment releases a peer's pinned snapshot without adding authority.
func syncAcknowledgment(revision uint64) *SOSyncMessage {
	return &SOSyncMessage{Body: &SOSyncMessage_Ack{Ack: &SOSyncAck{Revision: revision}}}
}

// prepareResponse reads a bounded suffix and serializes exactly the advertised state.
func (s *SOSync) prepareResponse(ctx context.Context, state *sobject.SOState, request *SOSyncHistoryRequest) (*syncResponse, error) {
	changes, err := s.soHost.ReadConfigHistory(ctx, request.GetBaseHash(), state.GetConfig().GetConfigChainHash())
	if err != nil {
		return nil, err
	}
	for _, change := range changes {
		if change.SizeVT()+256 > maxHistoryPageBytes {
			return nil, sobject.ErrConfigHistoryUnavailable
		}
	}

	// Invitations are local capabilities; peers import neither invitations nor nonce bookkeeping.
	state = state.CloneVT()
	state.Invites = nil
	state.QueuedAccountNonces = nil
	data, err := state.MarshalVT()
	if err != nil {
		return nil, err
	}
	snapshot := &SOSyncSnapshot{SoState: data, RootSeqno: state.GetRoot().GetInnerSeqno(), Revision: request.GetRevision(), BaseHash: bytes.Clone(request.GetBaseHash())}
	if (&SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: snapshot}}).SizeVT() > maxMessageSize {
		return nil, sobject.ErrConfigHistoryUnavailable
	}
	return &syncResponse{revision: request.GetRevision(), cursor: bytes.Clone(request.GetBaseHash()), changes: changes, snapshot: snapshot}, nil
}

// nextMessage emits one causal page or the final snapshot without exceeding a frame budget.
func (r *syncResponse) nextMessage() (*SOSyncMessage, error) {
	if len(r.changes) == 0 {
		return &SOSyncMessage{Body: &SOSyncMessage_Snapshot{Snapshot: r.snapshot}}, nil
	}
	page := &SOSyncHistoryPage{Revision: r.revision, Cursor: bytes.Clone(r.cursor)}
	message := &SOSyncMessage{Body: &SOSyncMessage_HistoryPage{HistoryPage: page}}
	for len(r.changes) != 0 && len(page.Changes) < maxHistoryPageEntries {
		page.Changes = append(page.Changes, r.changes[0])
		if message.SizeVT() > maxHistoryPageBytes {
			page.Changes = page.Changes[:len(page.Changes)-1]
			break
		}
		hash, err := sobject.HashSOConfigChange(r.changes[0])
		if err != nil {
			return nil, err
		}
		r.cursor = hash
		r.changes = r.changes[1:]
	}
	if len(page.Changes) == 0 {
		return nil, sobject.ErrConfigHistoryUnavailable
	}
	return message, nil
}

// appendPage checks target binding, cursor continuity and aggregate budgets.
func (r *syncReceive) appendPage(message *SOSyncMessage) error {
	page := message.GetHistoryPage()
	if page.GetRevision() != r.head.GetRevision() || !bytes.Equal(page.GetCursor(), r.cursor) || len(page.GetChanges()) == 0 {
		return errors.New("invalid history page")
	}
	if message.SizeVT() > maxHistoryPageBytes || len(page.GetChanges()) > maxHistoryPageEntries {
		return sobject.ErrConfigHistoryUnavailable
	}
	for _, change := range page.GetChanges() {
		if !bytes.Equal(change.GetPreviousHash(), r.cursor) {
			return errors.New("noncontiguous history page")
		}
		r.size += change.SizeVT()
		if r.size > sobject.MaxConfigSuffixBytes || len(r.changes) == sobject.MaxConfigSuffixEntries {
			return sobject.ErrConfigHistoryUnavailable
		}
		hash, err := sobject.HashSOConfigChange(change)
		if err != nil {
			return err
		}
		r.cursor = hash
		r.changes = append(r.changes, change)
	}
	return nil
}

// acceptResponse verifies the pinned response through the host's atomic import boundary.
func (s *SOSync) acceptResponse(ctx context.Context, receiving *syncReceive, snapshot *SOSyncSnapshot) error {
	if snapshot.GetRevision() != receiving.head.GetRevision() || !bytes.Equal(snapshot.GetBaseHash(), receiving.base) || !bytes.Equal(receiving.cursor, receiving.head.GetConfigHash()) {
		return errors.New("snapshot does not complete requested history")
	}
	digest := sha256.Sum256(snapshot.GetSoState())
	if !bytes.Equal(digest[:], receiving.head.GetStateHash()) {
		return errors.New("snapshot differs from advertised content digest")
	}
	if s.responseObsolete(ctx, receiving.head) {
		return nil
	}
	state := &sobject.SOState{}
	if err := state.UnmarshalVT(snapshot.GetSoState()); err != nil {
		return err
	}
	if snapshot.GetRootSeqno() != receiving.head.GetRootSeqno() || state.GetRoot().GetInnerSeqno() != snapshot.GetRootSeqno() || state.GetConfig().GetConfigChainSeqno() != receiving.head.GetConfigSeqno() || !bytes.Equal(state.GetConfig().GetConfigChainHash(), receiving.head.GetConfigHash()) {
		return errors.New("snapshot differs from pinned advertisement")
	}
	err := s.soHost.ImportPeerSnapshot(ctx, state, receiving.changes, s.localObjectPeerID, s.validateSnapshotAccess)
	if err != nil && s.responseObsolete(ctx, receiving.head) {
		return nil
	}
	return err
}

// syncStateHash detects unchanged content without treating the advertised digest as authority.
func syncStateHash(state *sobject.SOState) ([]byte, error) {
	if len(state.GetConfig().GetConfigChainHash()) == 0 {
		return nil, sobject.ErrConfigHistoryUnavailable
	}
	state = state.CloneVT()
	state.Invites = nil
	state.QueuedAccountNonces = nil
	if state.SizeVT() > maxMessageSize {
		return nil, sobject.ErrConfigHistoryUnavailable
	}
	data, err := state.MarshalVT()
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(data)
	return digest[:], nil
}

// responseObsolete permits declining a delayed response after local progress.
// Equal-sequence conflicting heads still require rejection at the host boundary.
func (s *SOSync) responseObsolete(ctx context.Context, head *SOSyncHead) bool {
	current, err := s.soHost.GetHostState(ctx)
	if err != nil {
		return false
	}
	configSeqno, rootSeqno := current.GetConfig().GetConfigChainSeqno(), current.GetRoot().GetInnerSeqno()
	return configSeqno >= head.GetConfigSeqno() && rootSeqno >= head.GetRootSeqno() &&
		(configSeqno > head.GetConfigSeqno() || rootSeqno > head.GetRootSeqno())
}
