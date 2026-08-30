package sobject_sync

import (
	"context"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// SyncProtocolID is the protocol ID used for SO sync solicitation.
const SyncProtocolID = protocol.ID("alpha/so-sync")

// maxMessageSize is the max message size for SO sync messages.
const maxMessageSize = 10 * 1024 * 1024

// SOSync manages bidirectional shared object state synchronization
// over a solicit protocol stream. Each instance syncs one SharedObject
// with peers connected via the session transport's child bus.
// SnapshotAccessValidator verifies that the local object identity can decode
// an inbound snapshot before it replaces durable state.
type SnapshotAccessValidator func(context.Context, *sobject.SOState) error

type SOSync struct {
	le                     *logrus.Entry
	b                      bus.Bus
	soID                   string
	localObjectPeerID      peer.ID
	soHost                 *sobject.SOHost
	validateSnapshotAccess SnapshotAccessValidator
}

// NewSOSync constructs a new SOSync.
//
// localObjectPeerID is the local storage identity checked against inbound
// state. The transport peer routes the sync stream but need not be a Space
// participant.
func NewSOSync(
	le *logrus.Entry,
	b bus.Bus,
	soID string,
	localObjectPeerID peer.ID,
	soHost *sobject.SOHost,
	accessValidators ...SnapshotAccessValidator,
) *SOSync {
	var validateSnapshotAccess SnapshotAccessValidator
	if len(accessValidators) != 0 {
		validateSnapshotAccess = accessValidators[0]
	}
	return &SOSync{
		le:                     le.WithField("so-sync", soID),
		b:                      b,
		soID:                   soID,
		localObjectPeerID:      localObjectPeerID,
		soHost:                 soHost,
		validateSnapshotAccess: validateSnapshotAccess,
	}
}

// Execute runs the SO sync, emitting a SolicitProtocol directive and
// handling matched streams until ctx is canceled.
func (s *SOSync) Execute(ctx context.Context) error {
	solicitCtx := []byte(s.soID)
	dir := link_solicit.NewSolicitProtocol(
		SyncProtocolID,
		solicitCtx,
		"",
		0,
	)

	_, solicitRef, err := s.b.AddDirective(
		dir,
		directive.NewTypedCallbackHandler[link_solicit.SolicitMountedStream](
			func(v directive.TypedAttachedValue[link_solicit.SolicitMountedStream]) {
				go s.handleSolicitedStream(ctx, v.GetValue())
			},
			nil, nil, nil,
		),
	)
	if err != nil {
		return err
	}
	defer solicitRef.Release()

	<-ctx.Done()
	return ctx.Err()
}

// handleSolicitedStream processes a matched solicit stream for SO sync.
func (s *SOSync) handleSolicitedStream(ctx context.Context, sms link_solicit.SolicitMountedStream) {
	ms, taken, err := sms.AcceptMountedStream()
	if err != nil || taken {
		return
	}

	strm := ms.GetStream()
	defer strm.Close()

	remotePeer := ms.GetPeerID().String()
	le := s.le.WithField("remote-peer", remotePeer)
	le.Debug("so sync stream accepted")

	sess := stream_packet.NewSession(strm, maxMessageSize)

	// Snapshot exchange: send our state, receive peer state.
	if err := s.exchangeSnapshots(ctx, le, sess); err != nil {
		if ctx.Err() == nil {
			le.WithError(err).Debug("so sync snapshot exchange failed")
		}
		return
	}

	// Bidirectional op streaming.
	s.streamOps(ctx, le, sess)
}

// exchangeSnapshots performs the initial snapshot exchange on the stream.
func (s *SOSync) exchangeSnapshots(ctx context.Context, le *logrus.Entry, sess *stream_packet.Session) error {
	// Get local state snapshot.
	localState, err := s.soHost.GetHostState(ctx)
	if err != nil {
		return err
	}

	localStateData, err := localState.MarshalVT()
	if err != nil {
		return err
	}

	localSeqno := localState.GetRoot().GetInnerSeqno()

	// Send our snapshot.
	outMsg := &SOSyncMessage{
		Body: &SOSyncMessage_Snapshot{
			Snapshot: &SOSyncSnapshot{
				SoState:   localStateData,
				RootSeqno: localSeqno,
			},
		},
	}
	if err := sess.SendMsg(outMsg); err != nil {
		return err
	}

	// Receive peer's snapshot.
	inMsg := &SOSyncMessage{}
	if err := sess.RecvMsg(inMsg); err != nil {
		return err
	}

	peerSnap := inMsg.GetSnapshot()
	if peerSnap == nil {
		return nil
	}
	return s.applyPeerSnapshot(ctx, le, peerSnap)
}

// applyPeerSnapshot validates and adopts a newer authoritative state.
func (s *SOSync) applyPeerSnapshot(
	ctx context.Context,
	le *logrus.Entry,
	peerSnap *SOSyncSnapshot,
) error {
	peerState := &sobject.SOState{}
	if err := peerState.UnmarshalVT(peerSnap.GetSoState()); err != nil {
		le.WithError(err).Warn("failed to unmarshal peer snapshot")
		return err
	}
	if peerState.GetRoot().GetInnerSeqno() != peerSnap.GetRootSeqno() {
		return errors.New("peer snapshot root sequence does not match its state")
	}
	if err := s.validateSnapshotElements(peerState); err != nil {
		le.WithError(err).Warn("rejected peer snapshot failing element validation")
		return errors.Wrap(err, "invalid peer snapshot")
	}
	if s.validateSnapshotAccess != nil {
		if err := s.validateSnapshotAccess(ctx, peerState); err != nil {
			le.WithError(err).Warn("rejected peer snapshot inaccessible to local object identity")
			return errors.Wrap(err, "inaccessible peer snapshot")
		}
	}

	var applied bool
	if err := s.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
		if peerSnap.GetRootSeqno() <= state.GetRoot().GetInnerSeqno() {
			return nil
		}
		merged := peerState.CloneVT()
		if err := mergePendingOperations(le, s.soID, merged, state); err != nil {
			return errors.Wrap(err, "preserve pending local operations")
		}
		*state = *merged
		applied = true
		return nil
	}); err != nil {
		le.WithError(err).Warn("failed to apply peer snapshot")
		return err
	}
	if applied {
		le.Debug("applied validated peer snapshot with higher seqno")
	}
	return nil
}

// mergePendingOperations carries unresolved operations that remain valid under
// the authoritative state. The authoritative configuration and queue capacity
// win when a pending local operation can no longer be admitted.
func mergePendingOperations(le *logrus.Entry, sharedObjectID string, authoritative, previous *sobject.SOState) error {
	rootNonces := make(map[string]uint64, len(authoritative.GetRoot().GetAccountNonces()))
	for _, account := range authoritative.GetRoot().GetAccountNonces() {
		rootNonces[account.GetPeerId()] = account.GetNonce()
	}
	for _, operation := range previous.GetOps() {
		inner, err := operation.UnmarshalInner()
		if err != nil {
			return err
		}
		if inner.GetNonce() <= rootNonces[inner.GetPeerId()] {
			continue
		}
		existing, rejection, err := authoritative.GetOperationStatus(inner.GetPeerId(), inner.GetLocalId())
		if err != nil {
			return err
		}
		if existing != nil || rejection != nil {
			continue
		}
		if err := authoritative.QueueOperation(sharedObjectID, operation); err != nil {
			le.WithError(err).WithFields(logrus.Fields{
				"operation-peer":  inner.GetPeerId(),
				"operation-nonce": inner.GetNonce(),
				"operation-id":    inner.GetLocalId(),
			}).Warn("dropping pending operation rejected by authoritative state")
			continue
		}
	}
	return nil
}

// validateSnapshotElements checks an inbound snapshot before it may replace
// local shared-object state. The sending peer may have self-authored the
// received config: nothing here verifies who wrote it or its lineage. The
// checks only require the local session peer to keep readable membership in
// the resulting config, and the grants and ops carried in the snapshot to
// carry internally valid signatures against that untrusted config.
//
// Full config-head authorization requires VerifyConfigChain over the
// sender's SOConfigChange entries. Neither SOSyncMessage nor any p2p-side
// store carries those entries today; closing that chain-lineage gap is a
// separate prerequisite.
func (s *SOSync) validateSnapshotElements(peerState *sobject.SOState) error {
	cfg := peerState.GetConfig()
	if cfg == nil || len(cfg.GetParticipants()) == 0 {
		return errors.New("snapshot has no participants")
	}

	localObjectPeerIDStr := s.localObjectPeerID.String()
	var localParticipant bool
	for _, p := range cfg.GetParticipants() {
		if p.GetPeerId() == localObjectPeerIDStr && sobject.CanReadState(p.GetRole()) {
			localParticipant = true
			break
		}
	}
	if !localParticipant {
		return errors.Errorf("local object peer %s is not a participant of the snapshot config", localObjectPeerIDStr)
	}

	for _, g := range peerState.GetRootGrants() {
		if err := g.ValidateSignature(s.soID, cfg.GetParticipants()); err != nil {
			return errors.Wrap(err, "root grant")
		}
	}
	for _, op := range peerState.GetOps() {
		if err := op.ValidateSignature(s.soID, cfg.GetParticipants()); err != nil {
			return errors.Wrap(err, "queued op")
		}
	}
	return nil
}

// streamOps runs bidirectional operation streaming until the context
// is canceled or the stream is closed.
func (s *SOSync) streamOps(ctx context.Context, le *logrus.Entry, sess *stream_packet.Session) {
	// Watch for local state changes and forward ops to peer.
	sendCtx, sendCancel := context.WithCancel(ctx)
	defer sendCancel()

	go s.sendOps(sendCtx, le, sess)

	// Receive ops from peer and apply.
	for {
		inMsg := &SOSyncMessage{}
		if err := sess.RecvMsg(inMsg); err != nil {
			if err != io.EOF && ctx.Err() == nil {
				le.WithError(err).Debug("so sync recv error")
			}
			return
		}

		switch body := inMsg.GetBody().(type) {
		case *SOSyncMessage_Snapshot:
			if err := s.applyPeerSnapshot(ctx, le, body.Snapshot); err != nil {
				return
			}
		case *SOSyncMessage_Op:
			s.handleRemoteOp(ctx, le, body.Op)
		case *SOSyncMessage_Ack:
			// Acknowledgment received, no action needed for MVP.
		}
	}
}

// sendOps watches for local state changes and sends new operations
// to the peer over the stream.
func (s *SOSync) sendOps(ctx context.Context, le *logrus.Entry, sess *stream_packet.Session) {
	stateCtr, relStateCtr, err := s.soHost.GetSOStateCtr(ctx, nil)
	if err != nil {
		return
	}
	defer relStateCtr()

	var (
		prev              *sobject.SOState
		lastSnapshotSeqno uint64
	)
	for {
		next, err := stateCtr.WaitValueChange(ctx, prev, nil)
		if err != nil {
			le.WithError(err).Debug("shared object state watch ended")
			return
		}

		rootSeqno := next.GetRoot().GetInnerSeqno()
		if rootSeqno > lastSnapshotSeqno {
			stateData, err := next.MarshalVT()
			if err != nil {
				le.WithError(err).Warn("failed to marshal snapshot for sync")
				return
			}
			if err := sess.SendMsg(&SOSyncMessage{
				Body: &SOSyncMessage_Snapshot{Snapshot: &SOSyncSnapshot{
					SoState:   stateData,
					RootSeqno: rootSeqno,
				}},
			}); err != nil {
				le.WithError(err).Debug("failed to send updated shared object snapshot")
				return
			}
			le.WithField("root-seqno", rootSeqno).Debug("sent updated shared object snapshot")
			lastSnapshotSeqno = rootSeqno
		}

		// Send operations queued since the authoritative root.
		for _, op := range next.GetOps() {
			opData, err := op.MarshalVT()
			if err != nil {
				le.WithError(err).Warn("failed to marshal op for sync")
				continue
			}

			msg := &SOSyncMessage{
				Body: &SOSyncMessage_Op{
					Op: &SOSyncOp{
						Operation: opData,
					},
				},
			}
			if err := sess.SendMsg(msg); err != nil {
				le.WithError(err).Debug("failed to send shared object operation")
				return
			}
		}

		prev = next
	}
}

// handleRemoteOp processes an operation received from the peer.
func (s *SOSync) handleRemoteOp(ctx context.Context, le *logrus.Entry, syncOp *SOSyncOp) {
	if len(syncOp.GetOperation()) == 0 {
		return
	}

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
