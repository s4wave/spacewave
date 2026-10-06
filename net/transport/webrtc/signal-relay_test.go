//go:build !js

package webrtc_test

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/signaling"
	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
)

// signalRelayEnd is one peer's end of an in-memory signaling relay. It is the
// controller that serves the peer's SignalPeer directives, and the session it
// serves for the other peer. While the end is down the relay drops every
// signal addressed to it, as a hosted relay does while the peer's signaling
// path is not ready, and reports each dropped offer.
type signalRelayEnd struct {
	// b is the peer bus the end serves.
	b bus.Bus
	// signalingID is the signaling channel the end serves.
	signalingID string
	// priv is the private key of the peer, to read the signals it drops.
	priv crypto.PrivKey
	// local is the peer ID of the end.
	local peer.ID
	// remote is the end of the other peer.
	remote *signalRelayEnd
	// inbox holds the signals delivered to the end.
	inbox chan []byte
	// down reports that the relay drops the signals addressed to the end.
	down atomic.Bool
	// droppedOffers receives a value for each offer dropped while down, if
	// nobody has read the previous one yet.
	droppedOffers chan struct{}
}

// newSignalRelay constructs the two ends of a relay between the peers.
func newSignalRelay(
	signalingID string,
	b0, b1 bus.Bus,
	priv0, priv1 crypto.PrivKey,
	id0, id1 peer.ID,
) (*signalRelayEnd, *signalRelayEnd) {
	newEnd := func(b bus.Bus, priv crypto.PrivKey, id peer.ID) *signalRelayEnd {
		return &signalRelayEnd{
			b:             b,
			signalingID:   signalingID,
			priv:          priv,
			local:         id,
			inbox:         make(chan []byte, 64),
			droppedOffers: make(chan struct{}, 1),
		}
	}
	end0, end1 := newEnd(b0, priv0, id0), newEnd(b1, priv1, id1)
	end0.remote, end1.remote = end1, end0
	return end0, end1
}

// GetControllerInfo returns information about the controller.
func (e *signalRelayEnd) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/signal-relay", controller.MustParseVersion("0.0.1"), "in-memory signaling relay")
}

// HandleDirective serves the session with the other peer.
func (e *signalRelayEnd) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(signaling.SignalPeer)
	if !ok || dir.SignalRemotePeerID() != e.remote.local {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]signaling.SignalPeerSession{e}), nil)
}

// Execute offers the session to the transport for incoming signals.
func (e *signalRelayEnd) Execute(ctx context.Context) error {
	// Offer the end as the handler of the session until the controller stops.
	di, ref, err := e.b.AddDirective(signaling.NewHandleSignalPeer(e.signalingID, e), nil)
	if err != nil {
		return err
	}
	defer di.Close()
	defer ref.Release()

	// Serve until the controller context ends.
	<-ctx.Done()
	return context.Canceled
}

// Close releases the controller.
func (e *signalRelayEnd) Close() error {
	return nil
}

// GetLocalPeerID returns the local peer ID.
func (e *signalRelayEnd) GetLocalPeerID() peer.ID {
	return e.local
}

// GetRemotePeerID returns the remote peer ID.
func (e *signalRelayEnd) GetRemotePeerID() peer.ID {
	return e.remote.local
}

// Send delivers msg to the other end, or drops it while that end is down.
func (e *signalRelayEnd) Send(ctx context.Context, msg []byte) error {
	// The sender scrubs msg after Send returns.
	msg = bytes.Clone(msg)
	to := e.remote
	if to.down.Load() {
		to.noteDropped(msg)
		return nil
	}
	select {
	case <-ctx.Done():
		return context.Canceled
	case to.inbox <- msg:
		return nil
	}
}

// Recv waits for a signal from the other end.
func (e *signalRelayEnd) Recv(ctx context.Context) ([]byte, error) {
	select {
	case <-ctx.Done():
		return nil, context.Canceled
	case msg := <-e.inbox:
		return msg, nil
	}
}

// noteDropped reports msg when it is an offer.
func (e *signalRelayEnd) noteDropped(msg []byte) {
	sig, err := webrtc.DecodeWebRtcSignal(msg, e.priv)
	if err != nil || sig.GetSdp().GetSdpType() != "offer" {
		return
	}
	select {
	case e.droppedOffers <- struct{}{}:
	default:
	}
}

// _ is a type assertion
var (
	_ controller.Controller       = (*signalRelayEnd)(nil)
	_ signaling.SignalPeerSession = (*signalRelayEnd)(nil)
)
