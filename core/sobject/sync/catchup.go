package sobject_sync

import (
	"bytes"
	"context"
	"crypto/sha256"
	"time"

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
		for {
			var message *SOSyncMessage
			select {
			case message = <-outbound:
			case <-ctx.Done():
				return ctx.Err()
			}
			err := s.authorizeParticipants(states.GetValue(), remoteID)
			if err == nil {
				err = sess.SendMsg(message)
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
		for _, worker := range workers {
			if exited, _ := worker.SetRoutine(nil); exited != nil {
				<-exited
			}
		}
	}()

	// Keep at most one advertisement, response, incoming suffix and control frame.
	var advertised, lastAdvertised *sobject.SOState
	var revision, remoteRevision uint64
	var advertisementDeadline time.Time
	var requested bool
	var receiving *syncReceive
	var response *syncResponse
	var control, outgoing, inFlight *SOSyncMessage
	var terminal error
	timer := time.NewTimer(catchupTimeout)
	timer.Stop()
	defer timer.Stop()
	for {
		// Controls precede response pages; a new head waits for the previous acknowledgment.
		current := states.GetValue()
		if err := s.authorizeParticipants(current, remoteID); err != nil {
			_ = sendAccessDenied(sess)
			return err
		}
		if outgoing == nil && inFlight == nil {
			switch {
			case control != nil:
				outgoing, control = control, nil
			case response != nil:
				outgoing, err = response.nextMessage()
				if err != nil {
					return err
				}
				if outgoing.GetSnapshot() != nil {
					response = nil
				}
			case advertised == nil && current != nil && !current.EqualVT(lastAdvertised):
				revision++
				if revision == 0 {
					return errors.New("sync revision exhausted")
				}
				digest, err := syncStateHash(current)
				if err != nil {
					return err
				}
				advertised, lastAdvertised = current, current
				advertisementDeadline = time.Now().Add(catchupTimeout)
				requested = false
				outgoing = &SOSyncMessage{Body: &SOSyncMessage_Head{Head: &SOSyncHead{
					Revision: revision, ConfigHash: bytes.Clone(current.GetConfig().GetConfigChainHash()),
					ConfigSeqno: current.GetConfig().GetConfigChainSeqno(), RootSeqno: current.GetRoot().GetInnerSeqno(), StateHash: digest,
				}}}
			}
		}

		// One timer covers both directions without extending the budget on each page.
		deadline := advertisementDeadline
		if receiving != nil && (deadline.IsZero() || receiving.deadline.Before(deadline)) {
			deadline = receiving.deadline
		}
		var expired <-chan time.Time
		if !deadline.IsZero() {
			timer.Reset(time.Until(deadline))
			expired = timer.C
		} else {
			timer.Stop()
		}
		var send chan *SOSyncMessage
		if outgoing != nil && inFlight == nil {
			send = outbound
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-expired:
			return context.DeadlineExceeded
		case <-changed:
			// The next iteration reads the latest authoritative state.
		case send <- outgoing:
			inFlight, outgoing = outgoing, nil
		case err := <-sent:
			if err != nil {
				return s.drainDenial(ctx, incoming, remoteID, err)
			}
			if inFlight.GetRecoveryRequired() != nil {
				return sobject.ErrConfigHistoryUnavailable
			}
			inFlight = nil
		case received := <-incoming:
			if received.err != nil {
				if terminal != nil {
					return terminal
				}
				return received.err
			}
			current = states.GetValue()
			if err := s.authorizeParticipants(current, remoteID); err != nil {
				_ = sendAccessDenied(sess)
				return err
			}
			if authorization, ok := received.message.GetBody().(*SOSyncMessage_Authorization); ok {
				return s.handleDenial(remoteID, authorization.Authorization)
			}

			// Drain already-sent frames without admitting new data after recovery becomes terminal.
			if terminal != nil {
				continue
			}

			switch body := received.message.GetBody().(type) {
			case *SOSyncMessage_Head:
				head := body.Head
				if len(head.GetConfigHash()) == 0 {
					return sobject.ErrConfigHistoryUnavailable
				}
				if head.GetRevision() <= remoteRevision || len(head.GetConfigHash()) > 128 || len(head.GetStateHash()) != sha256.Size || receiving != nil || control != nil {
					return errors.New("invalid or overlapping sync head")
				}
				remoteRevision = head.GetRevision()
				digest, err := syncStateHash(current)
				if err != nil {
					return err
				}
				if bytes.Equal(head.GetStateHash(), digest) && s.peerRecovery != nil {
					s.peerRecovery(remoteID, false)
				}
				config := current.GetConfig()
				if bytes.Equal(head.GetStateHash(), digest) || head.GetConfigSeqno() < config.GetConfigChainSeqno() ||
					(bytes.Equal(head.GetConfigHash(), config.GetConfigChainHash()) && head.GetRootSeqno() < current.GetRoot().GetInnerSeqno()) {
					control = syncAcknowledgment(head.GetRevision())
					continue
				}
				base := bytes.Clone(config.GetConfigChainHash())
				receiving = &syncReceive{head: head, base: base, cursor: base, deadline: time.Now().Add(catchupTimeout)}
				control = &SOSyncMessage{Body: &SOSyncMessage_HistoryRequest{HistoryRequest: &SOSyncHistoryRequest{
					Revision: head.GetRevision(), BaseHash: base,
				}}}
			case *SOSyncMessage_HistoryRequest:
				request := body.HistoryRequest
				if advertised == nil || request.GetRevision() != revision || requested || len(request.GetBaseHash()) == 0 || len(request.GetBaseHash()) > 128 {
					return errors.New("invalid or repeated sync request")
				}
				requested = true
				requestCtx, requestCancel := context.WithDeadline(ctx, advertisementDeadline)
				response, err = s.prepareResponse(requestCtx, advertised, request)
				requestCancel()
				if err != nil {
					terminal = sobject.ErrConfigHistoryUnavailable
					control = &SOSyncMessage{Body: &SOSyncMessage_RecoveryRequired{RecoveryRequired: &SOSyncRecoveryRequired{Revision: revision}}}
				}
			case *SOSyncMessage_HistoryPage:
				if receiving == nil {
					return errors.New("unsolicited history page")
				}
				if err := receiving.appendPage(received.message); err != nil {
					if !errors.Is(err, sobject.ErrConfigHistoryUnavailable) {
						return err
					}
					terminal = err
					control = &SOSyncMessage{Body: &SOSyncMessage_RecoveryRequired{RecoveryRequired: &SOSyncRecoveryRequired{Revision: receiving.head.GetRevision()}}}
				}
			case *SOSyncMessage_Snapshot:
				if receiving == nil || control != nil {
					return errors.Wrap(sobject.ErrConfigHistoryUnavailable, "peer did not use requested snapshot protocol")
				}
				requestCtx, requestCancel := context.WithDeadline(ctx, receiving.deadline)
				err := s.acceptResponse(requestCtx, receiving, body.Snapshot)
				requestCancel()
				if err != nil {
					return err
				}
				if s.peerRecovery != nil && !s.responseObsolete(ctx, receiving.head) {
					s.peerRecovery(remoteID, false)
				}
				control = syncAcknowledgment(receiving.head.GetRevision())
				receiving = nil
			case *SOSyncMessage_Ack:
				if advertised == nil || body.Ack.GetRevision() != revision || response != nil {
					return errors.New("invalid sync acknowledgment")
				}
				advertised = nil
				advertisementDeadline = time.Time{}
			case *SOSyncMessage_RecoveryRequired:
				matchesRequest := receiving != nil && body.RecoveryRequired.GetRevision() == receiving.head.GetRevision()
				matchesAdvertisement := advertised != nil && body.RecoveryRequired.GetRevision() == revision
				if !matchesRequest && !matchesAdvertisement {
					return errors.New("unsolicited recovery response")
				}
				return sobject.ErrConfigHistoryUnavailable
			case *SOSyncMessage_Op:
				s.handleRemoteOp(ctx, le, body.Op)
			default:
				return errors.New("unexpected authenticated sync message")
			}
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
