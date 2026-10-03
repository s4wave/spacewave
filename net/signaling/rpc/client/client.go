package signaling_rpc_client

import (
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/backoff"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	"github.com/sirupsen/logrus"
)

// Client implements a signaling service client.
// Tracks a set of ongoing Session RPCs.
// Manages backpressure on senders across the signaling channel.
// Manages validating and signing messages with the peer private key.
type Client struct {
	// le is the logger for the client.
	le *logrus.Entry
	// client is the signaling RPC client.
	client signaling_rpc.SRPCSignalingClient
	// privKey is the local private key.
	privKey crypto.PrivKey
	// peerID is the peer ID derived from the private key.
	peerID peer.ID

	// peers is keyed by peer id in string format
	peers *keyed.KeyedRefCount[string, *clientPeerTracker]
	// listenRoutine is the routine for the Listen RPC
	// only enabled if listenHandler != nil
	listenRoutine *routine.RoutineContainer
}

// ClientListenHandler is a function to handle when the incoming sessions list changes.
//
// pid is the peer id in string format.
type ClientListenHandler func(ctx context.Context, reset, added bool, pid string)

// NewClient constructs a new client.
func NewClient(
	le *logrus.Entry,
	c signaling_rpc.SRPCSignalingClient,
	privKey crypto.PrivKey,
	backoffConf *backoff.Backoff,
) (*Client, error) {
	// Derive the local signaling peer identity from its private key.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	// Construct the signaling client with its RPC service and peer identity.
	client := &Client{
		le:      le,
		client:  c,
		privKey: privKey,
		peerID:  peerID,
	}

	// Listen routine connects & waits for remote peers to contact us.
	client.listenRoutine = routine.NewRoutineContainer(
		routine.WithBackoff(backoffConf.Construct()),
		routine.WithExitLogger(le.WithField("routine", "signaling-client-listen")),
	)

	// Peer trackers start when we want to send or receive signals from/to a remote peer.
	client.peers = keyed.NewKeyedRefCount[string, *clientPeerTracker](
		client.newPeerTracker,
		keyed.WithExitLogger[string, *clientPeerTracker](le),
		keyed.WithBackoff[string, *clientPeerTracker](func(k string) cbackoff.BackOff {
			return backoffConf.Construct()
		}),
	)

	return client, nil
}

// NewClientWithBus constructs a new client that contacts the server via a Bifrost stream.
//
// If protocolID is empty, uses the default signaling protocol id.
// If serviceID is empty, uses the default signaling service id.
func NewClientWithBus(
	le *logrus.Entry,
	b bus.Bus,
	privKey crypto.PrivKey,
	clientConf *stream_srpc_client.Config,
	protocolID protocol.ID,
	serviceID string,
) (*Client, error) {
	// Apply default protocol and service identifiers.
	if protocolID == "" {
		protocolID = signaling_rpc.ProtocolID
	}

	// Choose the default signaling service when the caller omits its identifier.
	if serviceID == "" {
		serviceID = signaling_rpc.SRPCSignalingServiceID
	}

	// Connect the signaling service through the configured Bifrost stream client.
	signalRpcClient, err := stream_srpc_client.NewClient(le, b, clientConf, protocolID)
	if err != nil {
		return nil, err
	}
	signalRpcService := signaling_rpc.NewSRPCSignalingClientWithServiceID(signalRpcClient, serviceID)
	return NewClient(le, signalRpcService, privKey, clientConf.GetPerServerBackoff())
}

// SetContext sets the context for the client.
// Until this is called, the client will do nothing.
func (c *Client) SetContext(ctx context.Context) {
	c.peers.SetContext(ctx, true)
	c.listenRoutine.SetContext(ctx, true)
}

// ClearContext clears the context for the client.
func (c *Client) ClearContext() {
	c.peers.ClearContext()
	_ = c.listenRoutine.ClearContext()
}

// SetListenHandler sets the handler to call when the Listen RPC returns a peer to contact.
// If nil, disables the Listen RPC.
//
// listenHandler: if set, calls Listen and updates the handler when the list of
// remote peers that want a session with the local peer changes.
//
// listenHandler is called with reset=true when the list is cleared.
//
// SetContext must also be called to start the Listen RPC routine.
func (c *Client) SetListenHandler(listenHandler ClientListenHandler) {
	if listenHandler == nil {
		_, _ = c.listenRoutine.SetRoutine(nil)
	} else {
		c.listenRoutine.SetRoutine(func(ctx context.Context) error {
			return c.executeListenRoutine(ctx, listenHandler)
		})
	}
}

// executeListenRoutine is the routine to run the Listen RPC.
func (c *Client) executeListenRoutine(ctx context.Context, handler ClientListenHandler) error {
	// Open the listen stream and notify the handler when it closes.
	c.le.Debug("signaling: starting to listen for incoming sessions")
	strm, err := c.client.Listen(ctx, &signaling_rpc.ListenRequest{})
	if err != nil {
		return err
	}
	defer func() {
		_ = strm.Close()
		handler(ctx, true, false, "")
	}()

	// Forward remote session membership changes to the listen handler.
	for {
		// Receive the next signaling membership update from the listen stream.
		msg, err := strm.Recv()
		if err != nil {
			return err
		}

		// Notify the listen handler when a remote peer requests or drops a session.
		switch b := msg.GetBody().(type) {
		case *signaling_rpc.ListenResponse_SetPeer:
			if b.SetPeer != "" {
				c.le.
					WithField("remote-peer", b.SetPeer).
					Debug("signaling: remote peer wants a session")
				handler(ctx, false, true, b.SetPeer)
			}
		case *signaling_rpc.ListenResponse_ClearPeer:
			if b.ClearPeer != "" {
				c.le.
					WithField("remote-peer", b.ClearPeer).
					Debug("signaling: remote peer no longer wants a session")
				handler(ctx, false, false, b.ClearPeer)
			}
		}
	}
}

// ClientPeerRef is a reference to a client peer.
type ClientPeerRef struct {
	c   *Client
	ref *keyed.KeyedRef[string, *clientPeerTracker]
	tkr *clientPeerTracker
}

// GetLocalPeerID returns the local peer ID.
func (r *ClientPeerRef) GetLocalPeerID() peer.ID {
	return r.c.peerID
}

// GetRemotePeerID returns the remote peer ID.
func (r *ClientPeerRef) GetRemotePeerID() peer.ID {
	return r.tkr.peerID
}

// Send signs msg with the peer private key, waits for remote send capacity,
// transmits the message, and waits for the remote peer to acknowledge it.
// If context is canceled the message will also be canceled.
func (r *ClientPeerRef) Send(ctx context.Context, msg []byte) (_ *signaling_rpc.SessionMsg, outErr error) {
	// Sign the outgoing signaling message with a unique peer sequence number.
	tkr := r.tkr
	seqno := tkr.txNonce.Add(1)
	sessMsg, err := signaling_rpc.NewSessionMsg(r.c.privKey, hash.RecommendedHashType, msg, seqno)
	if err != nil {
		return nil, err
	}

	// Track delivery and withdraw a transmitted message when this send fails.
	var txed, acked bool
	defer func() {
		// Keep successful sends and messages that were never transmitted unchanged.
		if !txed || outErr == nil {
			return
		}

		// Clear acknowledged messages or request cancellation of an outstanding send.
		tkr.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if tkr.out != nil && tkr.out.Seqno == seqno {
				// If the message was already acknowledged, clear it.
				if !tkr.outSent || tkr.outAcked {
					tkr.out, tkr.outSent, tkr.outAcked, tkr.outCancel = nil, false, false, false
					broadcast()
				} else if !tkr.outCancel {
					// Otherwise mark to the main routine that we need to cancel this msg.
					tkr.outCancel = true
					broadcast()
				}
			}
		})
	}()

	// Wait for the signaling session to accept and acknowledge this message.
	for {
		// Reconcile message ownership and acknowledgement under the peer tracker lock.
		var waitCh <-chan struct{}
		tkr.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// Wait for an open signaling session before queuing this message.
			if tkr.open == nil {
				txed = false
				waitCh = getWaitCh()
				return
			}

			// Recover message ownership when the signaling session replaces its queue.
			if txed {
				if tkr.out == nil {
					txed = false
				} else if tkr.out.Seqno != seqno {
					txed = false
					waitCh = getWaitCh()
					return
				}
			}

			// Queue this message when the signaling peer has outgoing capacity.
			if !txed {
				if tkr.out == nil {
					txed = true
					tkr.out = sessMsg
					broadcast()
				}

				waitCh = getWaitCh()
				return
			}

			// Complete this send once the remote peer acknowledges its message.
			if tkr.outAcked {
				acked = true
				tkr.out, tkr.outSent, tkr.outAcked = nil, false, false
				broadcast()
				return
			}

			// Subscribe to the next signaling peer state change while still locked.
			waitCh = getWaitCh()
		})

		// Return the signed message once its acknowledgement has been observed.
		if acked {
			return sessMsg, nil
		}

		// Wait for signaling peer progress or caller cancellation.
		if waitCh != nil {
			select {
			case <-ctx.Done():
				return nil, context.Canceled
			case <-waitCh:
			}
		}
	}
}

// Recv waits for and acks an incoming message from a remote peer.
func (r *ClientPeerRef) Recv(ctx context.Context) (*signaling_rpc.SessionMsg, error) {
	tkr := r.tkr

	var recv *signaling_rpc.SessionMsg
	for {
		var waitCh <-chan struct{}
		tkr.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if tkr.recv == nil || tkr.recvProcessed {
				waitCh = getWaitCh()
				return
			}

			recv = tkr.recv
			tkr.recvProcessed = true
			broadcast()
		})

		if recv != nil {
			return recv, nil
		}

		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-waitCh:
		}
	}
}

// Release releases the peer reference.
func (r *ClientPeerRef) Release() {
	r.ref.Release()
}

// AddPeerRef adds a reference to a remote peer.
// Initiates a session with the remote peer that can send/recv messages.
// Be sure to release the ref when done with it.
func (c *Client) AddPeerRef(remotePeerID string) *ClientPeerRef {
	ref, tracker, _ := c.peers.AddKeyRef(remotePeerID)
	return &ClientPeerRef{c: c, ref: ref, tkr: tracker}
}

// clientPeerTracker wraps an ongoing signaling clientPeer with a peer.
type clientPeerTracker struct {
	// txNonce is the transmission nonce counter
	txNonce atomic.Uint64
	// c is the client
	c *Client
	// le is the logger
	le *logrus.Entry
	// key is the string encoding of the peer id.
	key string
	// peerID is the parsed version of the peer id
	peerID peer.ID
	// bcast guards below fields
	bcast broadcast.Broadcast
	// open indicates the session is open
	open *uint64
	// out contains the next message to send out.
	out *signaling_rpc.SessionMsg
	// outSent indicates out was sent to the server.
	outSent bool
	// outAcked indicates out was acked.
	outAcked bool
	// outCancel indicates we should try to cancel sending out.
	outCancel bool
	// recv contains the next message to receive.
	recv *signaling_rpc.SessionMsg
	// recvProcessed indicates that recv was processed.
	recvProcessed bool
}

// newPeerTracker constructs a new clientPeerTracker.
func (c *Client) newPeerTracker(peerIDStr string) (keyed.Routine, *clientPeerTracker) {
	// note: we confirmed that peerIDStr is valid before adding a key.
	peerID, err := peer.IDB58Decode(peerIDStr)
	if err != nil {
		return nil, nil
	}

	// Construct the peer tracker with a logger bound to its remote identity.
	le := c.le.WithField("remote-peer-id", peerIDStr)
	sess := &clientPeerTracker{
		c:      c,
		le:     le,
		key:    peerIDStr,
		peerID: peerID,
	}
	return sess.execute, sess
}

// execute executes the clientPeerTracker.
func (s *clientPeerTracker) execute(ctx context.Context) error {
	// Open the long-lived signaling session for this peer tracker.
	sess, err := s.c.client.Session(ctx)
	if err != nil {
		return errors.Wrap(err, "open signaling rpc session")
	}

	// Route session failures through the tracker error channel.
	errCh := make(chan error, 2)

	// Define state transitions for close, open, receive, and acknowledgements.
	handleClose := func() {
		s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if s.open != nil {
				s.open = nil
				broadcast()
			}
			if s.out != nil {
				s.out, s.outSent, s.outAcked, s.outCancel = nil, false, false, false
				broadcast()
			}
			if s.recv != nil {
				s.recv, s.recvProcessed = nil, false
				broadcast()
			}
		})
	}

	// Reset message delivery state when the remote signaling session opens.
	handleOpen := func(seqno uint64) {
		s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if s.open == nil || *s.open != seqno {
				s.le.Debugf("signaling: client: session opened with seqno %v", seqno)
				s.open = &seqno
				s.outAcked, s.outSent = false, false
				s.recv, s.recvProcessed = nil, false
				broadcast()
			}
		})
	}

	// Validate incoming signaling messages before publishing them to receivers.
	handleRecv := func(msg *signaling_rpc.SessionMsg) error {
		// Verify the incoming message signature and recover its peer identity.
		_, id, err := msg.ExtractAndVerify()
		if err != nil {
			return err
		}

		// Require the incoming message identity to match the tracked remote peer.
		expectedPeerIDStr := s.key
		actualPeerIDStr := id.String()
		if expectedPeerIDStr != actualPeerIDStr {
			return errors.Errorf("expected message peer id %s but got %s", expectedPeerIDStr, actualPeerIDStr)
		}

		// Publish the verified message and wake receivers under the tracker lock.
		s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// s.le.Debugf("signaling: client: recv msg: %v", msg.String())
			s.recv, s.recvProcessed = msg, false
			broadcast()
		})

		return nil
	}

	// Withdraw an incoming message when the remote peer clears its sequence number.
	handleClearMsg := func(msgSeqno uint64) {
		s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// s.le.Debugf("signaling: client: remote cleared msg: %v", msgSeqno)
			if s.recv != nil && s.recv.Seqno == msgSeqno {
				s.recv, s.recvProcessed = nil, false
				broadcast()
			}
		})
	}

	// Complete or discard the outgoing message when its acknowledgement arrives.
	handleAckMsg := func(msgSeqno uint64) {
		s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// s.le.Debugf("signaling: client: remote acked msg: %v", msgSeqno)
			if s.out != nil && s.out.Seqno == msgSeqno {
				if s.outCancel {
					s.out, s.outAcked, s.outCancel, s.outSent = nil, false, false, false
				} else {
					s.outAcked = true
				}
				broadcast()
			}
		})
	}

	// Close the session and clear all tracker state on exit.
	defer func() {
		_ = sess.Close()
		handleClose()
	}()

	// Register this peer identity with the remote signaling server.
	err = sess.Send(&signaling_rpc.SessionRequest{
		Body: &signaling_rpc.SessionRequest_Init{
			Init: &signaling_rpc.SessionInit{
				PeerId: s.key,
			},
		},
	})
	if err != nil {
		return errors.Wrap(err, "send signaling init")
	}

	// Consume remote session responses in a dedicated receiver.
	go func() {
		for {
			resp, err := sess.Recv()
			if err != nil {
				errCh <- err
				return
			}

			switch b := resp.GetBody().(type) {
			case *signaling_rpc.SessionResponse_Closed:
				if b.Closed {
					handleClose()
				}
			case *signaling_rpc.SessionResponse_Opened:
				handleOpen(b.Opened)
			case *signaling_rpc.SessionResponse_RecvMsg:
				if b.RecvMsg != nil {
					if err := handleRecv(b.RecvMsg); err != nil {
						errCh <- err
						return
					}
				}
			case *signaling_rpc.SessionResponse_AckMsg:
				handleAckMsg(b.AckMsg)
			case *signaling_rpc.SessionResponse_ClearMsg:
				handleClearMsg(b.ClearMsg)
			default:
				errCh <- errors.New("unrecognized SessionResponse message")
				return
			}
		}
	}()

	// Reconcile queued sends, cancellations, and acknowledgements.
	for {

		if err := ctx.Err(); err != nil {
			return context.Canceled
		}

		var waitCh <-chan struct{}
		var sendMsg *signaling_rpc.SessionMsg
		var cancelMsg uint64
		var ackRecvMsg uint64
		var sessSeqno uint64

		s.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if s.open != nil {
				sessSeqno = *s.open

				if s.out != nil && s.outCancel {
					cancelMsg = s.out.Seqno
					s.out, s.outAcked, s.outCancel, s.outSent = nil, false, false, false
					broadcast()
					return
				}

				if s.out != nil && !s.outSent {
					sendMsg = s.out
					s.outSent = true
					broadcast()
					return
				}

				if s.recv != nil && s.recvProcessed {
					ackRecvMsg = s.recv.Seqno
					s.recv = nil
					s.recvProcessed = false
					broadcast()
					return
				}
			}

			waitCh = getWaitCh()
		})

		// Acknowledge messages consumed by the caller.
		if ackRecvMsg != 0 {
			err := sess.Send(&signaling_rpc.SessionRequest{
				SessionSeqno: sessSeqno,
				Body: &signaling_rpc.SessionRequest_AckMsg{
					AckMsg: ackRecvMsg,
				},
			})
			if err != nil {
				return errors.Wrap(err, "send signaling ack")
			}
		}

		// Cancel messages withdrawn before transmission.
		if cancelMsg != 0 {
			err := sess.Send(&signaling_rpc.SessionRequest{
				SessionSeqno: sessSeqno,
				Body: &signaling_rpc.SessionRequest_ClearMsg{
					ClearMsg: cancelMsg,
				},
			})
			if err != nil {
				return errors.Wrap(err, "send signaling clear")
			}
		}

		// Transmit the next queued message.
		if sendMsg != nil {
			err := sess.Send(&signaling_rpc.SessionRequest{
				SessionSeqno: sessSeqno,
				Body: &signaling_rpc.SessionRequest_SendMsg{
					SendMsg: sendMsg,
				},
			})
			if err != nil {
				return errors.Wrap(err, "send signaling payload")
			}
		}

		if waitCh != nil {
			select {
			case <-ctx.Done():
				return context.Canceled
			case err := <-errCh:
				return err
			case <-waitCh:
			}
		}
	}
}
