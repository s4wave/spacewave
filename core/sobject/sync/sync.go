package sobject_sync

import (
	"context"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/sirupsen/logrus"
)

// SyncProtocolID is the protocol ID used for SO sync solicitation.
const SyncProtocolID = protocol.ID("alpha/so-sync")

// maxMessageSize is the max message size for SO sync messages.
const maxMessageSize = 10 * 1024 * 1024

// SOSync manages bidirectional shared object state synchronization
// over a solicit protocol stream. Each instance syncs one SharedObject
// with peers connected via the session transport's child bus.
type SOSync struct {
	le     *logrus.Entry
	b      bus.Bus
	soID   string
	soHost *sobject.SOHost
}

// NewSOSync constructs a new SOSync.
func NewSOSync(le *logrus.Entry, b bus.Bus, soID string, soHost *sobject.SOHost) *SOSync {
	return &SOSync{
		le:     le.WithField("so-sync", soID),
		b:      b,
		soID:   soID,
		soHost: soHost,
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

	// Compare root_seqno: if peer is newer, apply their full state.
	// The full state includes config (participants, grants) and root,
	// which is necessary for paired devices that share the same SO but
	// may have divergent configs until the first sync.
	if peerSnap.GetRootSeqno() > localSeqno {
		peerState := &sobject.SOState{}
		if err := peerState.UnmarshalVT(peerSnap.GetSoState()); err != nil {
			le.WithError(err).Warn("failed to unmarshal peer snapshot")
			return err
		}

		if err := s.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
			*state = *peerState
			return nil
		}); err != nil {
			le.WithError(err).Warn("failed to apply peer snapshot")
			return err
		}
		le.Debug("applied peer snapshot with higher seqno")
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

	var prev *sobject.SOState
	for {
		next, err := stateCtr.WaitValueChange(ctx, prev, nil)
		if err != nil {
			return
		}

		// Send new ops that appeared since last state.
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

	// Queue the signed operation directly against the SOHost.
	// The SOHost validates signatures and nonces.
	if err := s.soHost.QueueOperation(ctx, peerID, func(nonce uint64) (*sobject.SOOperation, error) {
		return op, nil
	}); err != nil {
		le.WithError(err).Debug("failed to queue remote op")
	}
}
