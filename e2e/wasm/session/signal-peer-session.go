package e2e_wasm_session

import (
	"context"
	"log"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
)

// signalPeerSession implements signaling.SignalPeerSession backed by a
// relaySession. Send/Recv delegate to the relay session channels.
type signalPeerSession struct {
	localPeerID  peer.ID
	remotePeerID peer.ID
	relay        *relaySession
}

// GetLocalPeerID returns the local peer ID.
func (s *signalPeerSession) GetLocalPeerID() peer.ID {
	return s.localPeerID
}

// GetRemotePeerID returns the remote peer ID.
func (s *signalPeerSession) GetRemotePeerID() peer.ID {
	return s.remotePeerID
}

// Send transmits a message to the remote peer via the relay.
func (s *signalPeerSession) Send(ctx context.Context, msg []byte) error {
	log.Printf("e2e signal relay send local=%s remote=%s bytes=%d", s.localPeerID.String(), s.remotePeerID.String(), len(msg))
	return s.relay.Send(ctx, msg)
}

// Recv waits for an incoming message from the remote peer via the relay.
func (s *signalPeerSession) Recv(ctx context.Context) ([]byte, error) {
	msg, err := s.relay.Recv(ctx)
	if err != nil {
		return nil, err
	}
	log.Printf("e2e signal relay recv local=%s remote=%s bytes=%d", s.localPeerID.String(), s.remotePeerID.String(), len(msg))
	return msg, nil
}

// signalPeerResolver resolves SignalPeer directives by looking up the relay
// session for the target remote peer.
type signalPeerResolver struct {
	c   *Controller
	dir signaling.SignalPeer
}

// Resolve resolves the SignalPeer directive by waiting for a relay session
// for the target remote peer, then emitting a SignalPeerSession value.
func (r *signalPeerResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Resolve the local peer from the bus.
	b := r.c.GetBus()
	p, _, ref, err := peer.GetPeerWithID(ctx, b, peer.ID(""), false, nil)
	if err != nil {
		return err
	}
	defer ref.Release()

	localPeerID := p.GetPeerID()

	// Check local peer ID filter.
	if dirLocal := r.dir.SignalLocalPeerID(); dirLocal != "" {
		if dirLocal.String() != localPeerID.String() {
			return nil
		}
	}

	remotePeerID := r.dir.SignalRemotePeerID()

	// Wait for a relay session targeting this remote peer.
	rs, err := r.c.relays.waitForSession(ctx, remotePeerID)
	if err != nil {
		return err
	}

	// Emit the SignalPeerSession value.
	sess := &signalPeerSession{
		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
		relay:        rs,
	}
	_, _ = handler.AddValue(signaling.SignalPeerValue(sess))
	return nil
}

// _ is a type assertion
var (
	_ signaling.SignalPeerSession = (*signalPeerSession)(nil)
	_ directive.Resolver          = (*signalPeerResolver)(nil)
)
