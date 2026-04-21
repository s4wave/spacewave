package signaling_rpc_server

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
)

// sessionTracker tracks a session between two peers.
type sessionTracker struct {
	// wait is a channel that is closed when below changes.
	wait chan struct{}
	// seqno is incremented when peerA or peerB changes
	seqno uint64
	// peerA is the tracker for peer A.
	// nil until connected
	peerA *sessionPeerTracker
	// peerB is the tracker for peer B.
	// nil until connected
	peerB *sessionPeerTracker
}

// sessionPeerTracker tracks a peer attached to a session.
type sessionPeerTracker struct {
	// recv contains a pending packet to receive.
	recv *signaling.SessionMsg
	// recvSent contains the seqno of the current packet sent but not acked.
	// if set, recv is nil.
	recvSent *uint64
	// recvClear indicates the remote end cleared a packet w/o acking.
	// this clear will be transmitted to the local peer
	recvClear *uint64
	// outAcked indicates a remote peer acked a packet.
	// this ack will be transmitted to the local peer
	outAcked *uint64
}

// getWaitCh returns the wait channel.
func (t *sessionTracker) getWaitCh() <-chan struct{} {
	wait := t.wait
	if wait == nil {
		wait = make(chan struct{})
		t.wait = wait
	}
	return wait
}

// broadcast closes the wait channel if any
func (t *sessionTracker) broadcast() {
	if t.wait != nil {
		close(t.wait)
		t.wait = nil
	}
}

// getCurrPeers returns the current peers for the session.
func (t *sessionTracker) getCurrPeers(srcIsPeerA bool) (currSrcPeer, currDstPeer *sessionPeerTracker) {
	if srcIsPeerA {
		currSrcPeer, currDstPeer = t.peerA, t.peerB
	} else {
		currSrcPeer, currDstPeer = t.peerB, t.peerA
	}
	return
}

// checkSeqno checks if a seqno is equal to the current and is a reasonable value.
func (t *sessionTracker) checkSeqno(seqno uint64) (bool, error) {
	// If the session sequence number is lower than the message session seqno, the peer is misbehaving.
	if t.seqno < seqno {
		return false, errors.Errorf("signaling: message session seqno is too high: %v > expected %v", seqno, t.seqno)
	}
	return t.seqno == seqno, nil
}

// sessionKey is the key for the sessions map
type sessionKey struct {
	// peerA is the lower-sorted peer ID string
	peerA, peerB string
}

// newSessionKey constructs a session key from two session peer ids.
// returns true if p1 is peer A and false if p1 is peer B.
func newSessionKey(p1, p2 string) (sessionKey, bool) {
	if strings.Compare(p1, p2) < 0 {
		return sessionKey{peerA: p1, peerB: p2}, true
	}
	return sessionKey{peerA: p2, peerB: p1}, false
}

// Session opens a session with a remote peer.
func (s *Server) Session(strm signaling.SRPCSignaling_SessionStream) error {
	ctx := strm.Context()
	srcPeerID, err := s.ident(ctx)
	if err != nil {
		return err
	}
	srcPeerIDStr := srcPeerID.String()

	// Wait for the init message.
	le := s.le.WithField("src-peer", srcPeerIDStr)
	req, err := strm.Recv()
	if err == nil && req.GetSessionSeqno() != 0 {
		err = errors.New("session seqno must be zero in init packet")
	}
	if err != nil {
		le.WithError(err).Warn("invalid init packet")
		return err
	}

	peerID, err := req.GetInit().ParsePeerID()
	if err != nil {
		le.WithError(err).Warn("invalid destination peer id")
		return err
	}
	if len(peerID) == 0 {
		le.Warn("invalid init with empty peer id")
		return errors.New("signaling: session expects init with peer id")
	}
	dstPeerIDStr := peerID.String()
	if dstPeerIDStr == srcPeerIDStr {
		le.Warn("signaling: self dial")
		return errors.New("signaling: cannot open session with self")
	}

	le = le.WithField("dst-peer", dstPeerIDStr)
	le.Debug("signaling: server: starting session")
	sessKey, localIsPeerA := newSessionKey(srcPeerIDStr, dstPeerIDStr)
	ourPeerTkr := &sessionPeerTracker{}

	// Lock and register initial state.
	s.mtx.Lock()

	// Register that we want a session with that peer.
	dstPeer, _ := s.getPeer(dstPeerIDStr)
	if _, ok := dstPeer.wantPeers[srcPeerIDStr]; !ok {
		dstPeer.wantPeers[srcPeerIDStr] = struct{}{}
		dstPeer.broadcast()
	}

	// Register our end of the session.
	sess, _ := s.getSession(sessKey)
	if localIsPeerA {
		sess.peerA = ourPeerTkr
	} else {
		sess.peerB = ourPeerTkr
	}
	// If there was a remote attached previously, clear any pending state.
	if _, prevRemotePeer := sess.getCurrPeers(localIsPeerA); prevRemotePeer != nil {
		prevRemotePeer.recv, prevRemotePeer.recvSent = nil, nil
	}

	sess.seqno++
	sess.broadcast()
	waitCh := sess.getWaitCh()

	s.mtx.Unlock()

	// Cleanup when we return.
	defer func() {
		s.mtx.Lock()
		// Check if we are still the active Session and clear out if so.
		var currLocalPeer **sessionPeerTracker
		if localIsPeerA {
			currLocalPeer = &sess.peerA
		} else {
			currLocalPeer = &sess.peerB
		}
		if *currLocalPeer == ourPeerTkr {
			// Clear the peer from the session and increment nonce.
			*currLocalPeer = nil
			// Get the current remote peer.
			if _, currRemotePeer := sess.getCurrPeers(localIsPeerA); currRemotePeer != nil {
				// Clear the pending packet to recv if any.
				currRemotePeer.recv, currRemotePeer.recvSent = nil, nil
			}
			sess.seqno++
			sess.broadcast()
			// Maybe drop the session if there's no peers.
			if s.maybeReleaseSession(sessKey) {
				defer le.Debug("signaling: server: finished session")
			}
			// Delete the session want from the remote peer.
			delete(dstPeer.wantPeers, srcPeerIDStr)
			dstPeer.broadcast()
			// Release the peer if there is no want & no session.
			_ = s.maybeReleasePeer(dstPeerIDStr)
		}
		s.mtx.Unlock()
	}()

	// Function to handle when our peer tries to send an outgoing message.
	handleSendMsg := func(msgSessionSeqno uint64, sendMsg *signaling.SessionMsg) error {
		// Verify signature on message and that the source peer matches.
		_, msgPeerId, err := sendMsg.ExtractAndVerify()
		if err != nil {
			return errors.Errorf("signaling: failed to verify signed msg: %v", err.Error())
		}
		msgPeerIdStr := msgPeerId.String()
		if msgPeerIdStr != srcPeerIDStr {
			return errors.Errorf("signaling: outgoing msg peer id mismatch: %v != expected %v", msgPeerId, srcPeerIDStr)
		}

		// Mark the outgoing message.
		s.mtx.Lock()
		defer s.mtx.Unlock()

		// If the sequence number is wrong, drop the packet.
		seqnoCurrent, err := sess.checkSeqno(msgSessionSeqno)
		if err != nil || !seqnoCurrent {
			return err
		}

		// Check if we are still the local peer.
		currLocalPeer, currRemotePeer := sess.getCurrPeers(localIsPeerA)
		if currLocalPeer != ourPeerTkr || currRemotePeer == nil {
			return nil
		}

		// Send the msg to the remote peer.
		currRemotePeer.recv = sendMsg
		currRemotePeer.recvSent = nil
		sess.broadcast()
		return nil
	}

	// Function to handle when our peer acks an incoming message.
	handleAckMsg := func(msgSessionSeqno, ack uint64) error {
		s.mtx.Lock()
		defer s.mtx.Unlock()

		// If the sequence number is wrong, drop the packet.
		seqnoCurrent, err := sess.checkSeqno(msgSessionSeqno)
		if err != nil || !seqnoCurrent {
			return err
		}

		// Transmit the ack if the message matches.
		currLocalPeer, currRemotePeer := sess.getCurrPeers(localIsPeerA)
		if currLocalPeer == ourPeerTkr && currRemotePeer != nil {
			if currLocalPeer.recvSent != nil && *currLocalPeer.recvSent == ack {
				currLocalPeer.recvSent = nil
				currRemotePeer.outAcked = &ack
				sess.broadcast()
			}
		}

		return nil
	}

	// Function to handle when our peer clears an outgoing message.
	handleClearMsg := func(msgSessionSeqno, clear uint64) error {
		s.mtx.Lock()
		defer s.mtx.Unlock()

		// If the sequence number is wrong, drop the packet.
		seqnoCurrent, err := sess.checkSeqno(msgSessionSeqno)
		if err != nil || !seqnoCurrent {
			return err
		}

		// Transmit the clear if the message matches.
		currLocalPeer, currRemotePeer := sess.getCurrPeers(localIsPeerA)
		if currLocalPeer == ourPeerTkr && currRemotePeer != nil {
			if currRemotePeer.recv != nil && currRemotePeer.recv.Seqno == clear {
				// We didn't transmit the message yet, drop it.
				currRemotePeer.recv = nil
			} else if currRemotePeer.recvSent != nil && *currRemotePeer.recvSent == clear {
				// Transmit the clear
				currRemotePeer.recvSent = nil
				currRemotePeer.recvClear = &clear
			}
		}

		return nil
	}

	// Start read goroutine.
	errCh := make(chan error, 2)
	go func() {
		for {
			msg, err := strm.Recv()
			if err != nil {
				errCh <- err
				return
			}

			// le.Debugf("signaling: server: got packet: %v", msg.String())
			sessSeqno := msg.GetSessionSeqno()
			switch bdy := msg.GetBody().(type) {
			case *signaling.SessionRequest_AckMsg:
				err = handleAckMsg(sessSeqno, bdy.AckMsg)
			case *signaling.SessionRequest_SendMsg:
				err = handleSendMsg(sessSeqno, bdy.SendMsg)
			case *signaling.SessionRequest_ClearMsg:
				err = handleClearMsg(sessSeqno, bdy.ClearMsg)
			default:
				err = signaling.ErrUnexpectedSessionMsg
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()

	// Start write / monitor loop.
	var prevSentOpenToLocal *uint64 // Tracks if we have sent open=true to the local peer.
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		case <-waitCh:
		}

		s.mtx.Lock()
		// Check if we are still the active session for this key.
		currLocalPeer, currRemotePeer := sess.getCurrPeers(localIsPeerA)
		currUserped := currLocalPeer != ourPeerTkr
		var currOpen *uint64
		if currRemotePeer != nil {
			currOpen = &sess.seqno
		}
		waitCh = sess.getWaitCh()

		// If we aren't userped & stream is open check if there is a message to send or clear.
		var msgToRecv *signaling.SessionMsg
		var msgToAck *uint64
		var msgToClear *uint64
		if !currUserped && currOpen != nil {
			// Get the message the remote peer wants to send out or clear or ack.
			msgToRecv, msgToClear, msgToAck = currLocalPeer.recv, currLocalPeer.recvClear, currLocalPeer.outAcked
			currLocalPeer.recv, currLocalPeer.recvClear, currLocalPeer.outAcked = nil, nil, nil

			// Mark as transmitted, if any.
			if msgToRecv != nil {
				seqno := msgToRecv.GetSeqno()
				currLocalPeer.recvSent = &seqno
				sess.broadcast()
			}
		}
		s.mtx.Unlock()

		// If userped, return.
		if currUserped {
			return signaling.ErrUserpedSession
		}

		// Send the opened or closed message if opened or closed.
		if prevSentOpenToLocal != currOpen {
			var err error
			if currOpen != nil {
				err = strm.Send(&signaling.SessionResponse{
					Body: &signaling.SessionResponse_Opened{Opened: *currOpen},
				},
				)
			} else {
				err = strm.Send(&signaling.SessionResponse{
					Body: &signaling.SessionResponse_Closed{Closed: true},
				},
				)
			}
			if err != nil {
				return err
			}
			prevSentOpenToLocal = currOpen
		}

		// Nothing else to do if not open.
		if currOpen == nil {
			continue
		}

		// Ack msg
		if msgToAck != nil {
			// le.Debugf("signaling: server: sending ack msg: %v", *msgToAck)
			if err := strm.Send(&signaling.SessionResponse{
				Body: &signaling.SessionResponse_AckMsg{AckMsg: *msgToAck},
			}); err != nil {
				return err
			}
		}

		// Clear msg
		if msgToClear != nil {
			// le.Debugf("signaling: server: sending clear msg: %v", *msgToClear)
			if err := strm.Send(&signaling.SessionResponse{
				Body: &signaling.SessionResponse_ClearMsg{ClearMsg: *msgToClear},
			}); err != nil {
				return err
			}
		}

		// Tx message
		if msgToRecv != nil {
			// le.Debugf("signaling: server: sending tx msg: %v", msgToRecv.String())
			if err := strm.Send(&signaling.SessionResponse{
				Body: &signaling.SessionResponse_RecvMsg{RecvMsg: msgToRecv},
			}); err != nil {
				return err
			}
		}
	}
}

// getSession gets or creates a peer tracker for a session key.
func (s *Server) getSession(sess sessionKey) (*sessionTracker, bool) {
	tkr, exists := s.sessions[sess]
	if !exists {
		tkr = &sessionTracker{}
		s.sessions[sess] = tkr
	}
	return tkr, exists
}

// maybeReleaseSession releases the session tracker if it has no references.
// returns if it was found & released
func (s *Server) maybeReleaseSession(sess sessionKey) bool {
	tkr := s.sessions[sess]
	if tkr == nil || tkr.peerA != nil || tkr.peerB != nil {
		return false
	}
	delete(s.sessions, sess)
	tkr.broadcast()
	return true
}
