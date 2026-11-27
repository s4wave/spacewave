package signaling_rpc_server

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
)

// sessionPeerTracker tracks a peer attached to a session.
type sessionPeerTracker struct {
	// recv contains a pending packet to receive.
	recv *signaling.SessionMsg
	// recvSent contains the seqno of the current packet sent but not acked.
	// if set, recv is nil.
	recvSent *uint64
	// recvClear indicates the remote end cleared a packet w/o acking.
	// this clear will be transmitted to the local peer.
	recvClear *uint64
	// outAcked indicates a remote peer acked a packet.
	// this ack will be transmitted to the local peer.
	outAcked *uint64
}

// sessionKey is the key for the sessions map.
type sessionKey struct {
	// peerA and peerB are the lower-sorted peer ID string and the other
	// peer, respectively.
	peerA, peerB string
}

// newSessionKey constructs a session key from two session peer ids.
// Returns true if p1 is peer A and false if p1 is peer B.
func newSessionKey(p1, p2 string) (sessionKey, bool) {
	if strings.Compare(p1, p2) < 0 {
		return sessionKey{peerA: p1, peerB: p2}, true
	}
	return sessionKey{peerA: p2, peerB: p1}, false
}

// Session opens a session with a remote peer.
func (s *Server) Session(strm signaling.SRPCSignaling_SessionStream) error {
	// Bind the stream to its authenticated source peer.
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

	// Resolve the destination before registering either session endpoint.
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

	// Create the endpoint identity used to fence replacement and cleanup.
	le = le.WithField("dst-peer", dstPeerIDStr)
	le.Debug("signaling: server: starting session")
	sessKey, localIsPeerA := newSessionKey(srcPeerIDStr, dstPeerIDStr)
	ourPeerTkr := &sessionPeerTracker{}

	// Lock and register initial state.
	s.mtx.Lock()

	// Register that the source peer wants a session with the destination
	// peer.
	dstPeer, _ := s.getPeer(dstPeerIDStr)
	dstPeer.addWant(srcPeerIDStr)

	// Claim the source endpoint and invalidate control state from the
	// previous generation.
	sess, _ := s.getSession(sessKey)
	sess.claimPeer(localIsPeerA, ourPeerTkr)
	s.mtx.Unlock()

	// Clean up when Session returns.
	defer func() {
		// Serialize session cleanup with endpoint replacement.
		s.mtx.Lock()

		// Release only the endpoint still attached to this stream.
		if sess.releasePeer(localIsPeerA, ourPeerTkr) {
			// Maybe drop the session if there's no peers.
			if s.maybeReleaseSession(sessKey) {
				defer le.Debug("signaling: server: finished session")
			}
			// Delete the session want from the remote peer.
			dstPeer.removeWant(srcPeerIDStr)
			// Release the peer if there is no want & no session.
			_ = s.maybeReleasePeer(dstPeerIDStr)
		}
		s.mtx.Unlock()
	}()

	// Handle when the source peer tries to send an outgoing message.
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

		// Check if the source endpoint is still the local peer.
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

	// Handle when the source peer acks an incoming message.
	handleAckMsg := func(msgSessionSeqno, ack uint64) error {
		// Serialize session acknowledgment updates with endpoint replacement.
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

	// Handle when the source peer clears an outgoing message.
	handleClearMsg := func(msgSessionSeqno, clear uint64) error {
		// Serialize session cancellation updates with endpoint replacement.
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
				// Drop the message because the session has not
				// transmitted the message yet.
				currRemotePeer.recv = nil
			} else if currRemotePeer.recvSent != nil && *currRemotePeer.recvSent == clear {
				// Transmit the clear request.
				currRemotePeer.recvSent = nil
				currRemotePeer.recvClear = &clear
			}
			sess.broadcast()
		}

		return nil
	}

	// Start the read goroutine.
	errCh := make(chan error, 2)
	go func() {
		for {
			msg, err := strm.Recv()
			if err != nil {
				errCh <- err
				return
			}

			// Apply the request only to its announced session generation.
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

	// Reconcile endpoint state before waiting for another change. Zero denotes
	// a closed session. Copy the sequence value so replacement remains
	// visible even when the session never transitions through an observed
	// closed state.
	var waitCh <-chan struct{}
	var prevSentOpenToLocal uint64
	for {
		s.mtx.Lock()
		// Check if the source endpoint is still the active session for
		// this key.
		currLocalPeer, currRemotePeer := sess.getCurrPeers(localIsPeerA)
		currUsurped := currLocalPeer != ourPeerTkr
		var currOpen uint64
		if currRemotePeer != nil {
			currOpen = sess.seqno
		}
		waitCh = sess.getWaitCh()

		// Snapshot pending protocol messages for this active endpoint.
		var msgToRecv *signaling.SessionMsg
		var msgToAck *uint64
		var msgToClear *uint64
		if !currUsurped && currOpen != 0 {
			// Get the message the remote peer wants to send out or clear or ack.
			msgToRecv, msgToClear, msgToAck = currLocalPeer.recv, currLocalPeer.recvClear, currLocalPeer.outAcked
			currLocalPeer.recv, currLocalPeer.recvClear, currLocalPeer.outAcked = nil, nil, nil

			// Mark the message as transmitted, if any.
			if msgToRecv != nil {
				seqno := msgToRecv.GetSeqno()
				currLocalPeer.recvSent = &seqno
				sess.broadcast()
			}
		}
		s.mtx.Unlock()

		// A replacement endpoint owns the session from this point forward.
		if currUsurped {
			return signaling.ErrUserpedSession
		}

		// Announce every generation change before delivering its messages.
		if prevSentOpenToLocal != currOpen {
			var err error
			if currOpen != 0 {
				err = strm.Send(&signaling.SessionResponse{
					Body: &signaling.SessionResponse_Opened{Opened: currOpen},
				})
			} else {
				err = strm.Send(&signaling.SessionResponse{
					Body: &signaling.SessionResponse_Closed{Closed: true},
				})
			}
			if err != nil {
				return err
			}
			prevSentOpenToLocal = currOpen
		}

		// Forward the pending acknowledgment, cancellation, and payload in order.
		if currOpen != 0 {
			// Acknowledge the message.
			if msgToAck != nil {
				if err := strm.Send(&signaling.SessionResponse{
					Body: &signaling.SessionResponse_AckMsg{AckMsg: *msgToAck},
				}); err != nil {
					return err
				}
			}

			// Clear the message.
			if msgToClear != nil {
				if err := strm.Send(&signaling.SessionResponse{
					Body: &signaling.SessionResponse_ClearMsg{ClearMsg: *msgToClear},
				}); err != nil {
					return err
				}
			}

			// Transmit the message.
			if msgToRecv != nil {
				if err := strm.Send(&signaling.SessionResponse{
					Body: &signaling.SessionResponse_RecvMsg{RecvMsg: msgToRecv},
				}); err != nil {
					return err
				}
			}
		}

		// Resume on endpoint changes, incoming protocol errors, or shutdown.
		select {
		case <-ctx.Done():
			return context.Canceled
		case err := <-errCh:
			return err
		case <-waitCh:
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
// Returns if it was found and released.
func (s *Server) maybeReleaseSession(sess sessionKey) bool {
	// Require both signaling endpoints to detach before releasing the session.
	tkr := s.sessions[sess]
	if tkr == nil || tkr.peerA != nil || tkr.peerB != nil {
		return false
	}

	// Remove the detached session tracker and wake its waiters.
	delete(s.sessions, sess)
	tkr.broadcast()
	return true
}
