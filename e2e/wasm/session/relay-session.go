package e2e_wasm_session

import (
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// relaySession holds the channel pair for a single signaling relay. Messages
// from the Go test arrive on incoming and are read by the SignalPeer resolver.
// Messages from the SignalPeer resolver are written to outgoing and forwarded
// to the Go test by the SignalRelay stream handler.
type relaySession struct {
	remotePeer peer.ID
	incoming   chan []byte
	outgoing   chan []byte
	ctx        context.Context
	cancel     context.CancelFunc
}

// newRelaySession creates a relay session for the given remote peer.
func newRelaySession(ctx context.Context, remotePeer peer.ID) *relaySession {
	ctx, cancel := context.WithCancel(ctx)
	return &relaySession{
		remotePeer: remotePeer,
		incoming:   make(chan []byte, 16),
		outgoing:   make(chan []byte, 16),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Send writes a signaling message to be forwarded to the Go test.
func (rs *relaySession) Send(ctx context.Context, msg []byte) error {
	msg = slices.Clone(msg)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-rs.ctx.Done():
		return errors.New("relay session closed")
	case rs.outgoing <- msg:
		return nil
	}
}

// Recv reads a signaling message from the Go test.
func (rs *relaySession) Recv(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-rs.ctx.Done():
		return nil, errors.New("relay session closed")
	case msg := <-rs.incoming:
		return msg, nil
	}
}

// Close cancels the relay session.
func (rs *relaySession) Close() {
	rs.cancel()
}

// relayRegistry tracks active relay sessions keyed by remote peer ID.
type relayRegistry struct {
	bcast    broadcast.Broadcast
	sessions map[string]*relaySession
}

// newRelayRegistry creates a new relay registry.
func newRelayRegistry() *relayRegistry {
	return &relayRegistry{sessions: make(map[string]*relaySession)}
}

// register adds a relay session for the given remote peer. Returns an error
// if a session for that peer already exists.
func (r *relayRegistry) register(rs *relaySession) error {
	key := string(rs.remotePeer)
	var err error
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if _, exists := r.sessions[key]; exists {
			err = errors.Errorf("relay session already exists for peer %s", rs.remotePeer.String())
			return
		}
		r.sessions[key] = rs
		broadcast()
	})
	return err
}

// unregister removes the relay session for the given remote peer.
func (r *relayRegistry) unregister(remotePeer peer.ID) {
	key := string(remotePeer)
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(r.sessions, key)
		broadcast()
	})
}

// lookup returns the relay session for the given remote peer, or nil.
func (r *relayRegistry) lookup(remotePeer peer.ID) *relaySession {
	key := string(remotePeer)
	var rs *relaySession
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		rs = r.sessions[key]
	})
	return rs
}

// waitForSession blocks until a relay session for the given remote peer is
// registered, or the context is canceled.
func (r *relayRegistry) waitForSession(ctx context.Context, remotePeer peer.ID) (*relaySession, error) {
	key := string(remotePeer)
	for {
		var rs *relaySession
		var waitCh <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			rs = r.sessions[key]
			if rs == nil {
				waitCh = getWaitCh()
			}
		})
		if rs != nil {
			return rs, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
	}
}
