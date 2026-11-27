package signaling_rpc_client

import (
	"context"

	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	signaling_rpc "github.com/s4wave/spacewave/net/signaling/rpc"
)

// ClientPeerRef is a reference to a client peer.
type ClientPeerRef struct {
	// c is the signaling client.
	c *Client
	// ref holds the peer tracker.
	ref *keyed.KeyedRef[string, *clientPeerTracker]
	// tkr is the peer tracker.
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
					// Otherwise notify the main routine
					// to cancel this msg.
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
	// Take the next unprocessed message, or wait for one.
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

		// Return the message once taken.
		if recv != nil {
			return recv, nil
		}

		// Wait for the next message or caller cancellation.
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-waitCh:
		}
	}
}

// GetPresence returns the presence epoch of the remote peer and a channel
// closed when the epoch may have changed. The epoch is zero while the remote
// peer is absent from the signaling session.
func (r *ClientPeerRef) GetPresence() (epoch uint64, changed <-chan struct{}) {
	r.tkr.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if r.tkr.open != nil {
			epoch = r.tkr.presence
		}
		changed = getWaitCh()
	})
	return epoch, changed
}

// Release releases the peer reference.
func (r *ClientPeerRef) Release() {
	r.ref.Release()
}
