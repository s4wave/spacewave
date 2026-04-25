package transport

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/s4wave/spacewave/net/transport/webrtc"
	"github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

// SessionTransport manages a session-scoped child bus with bifrost
// transport controllers bound to the session's peer identity.
type SessionTransport struct {
	le *logrus.Entry
	// parentBus is the parent controller bus to bridge directives to.
	parentBus bus.Bus
	// childBus is the session-scoped child bus.
	childBus bus.Bus
	// sessionKey is the session's Ed25519 private key.
	sessionKey bifrost_crypto.PrivKey
	// peerID is the peer ID derived from the session key.
	peerID peer.ID
	// signalingURL is the cloud API base URL (e.g. "https://alpha.spacewave.app").
	signalingURL string
	// signingEnvPfx is the request-signing environment prefix.
	signingEnvPfx string
	// ready is closed when the child bus is created and base controllers started.
	ready chan struct{}
}

// NewSessionTransport constructs a new session-scoped transport.
//
// The child bus is created in Execute. The sessionKey is the session's
// Ed25519 private key used as the transport peer identity.
//
// signalingURL is the cloud API base URL for the SignalingDO endpoint.
// If empty, WebRTC and signaling controllers are not started.
func NewSessionTransport(
	le *logrus.Entry,
	parentBus bus.Bus,
	sessionKey bifrost_crypto.PrivKey,
	signalingURL string,
	signingEnvPfx string,
) (*SessionTransport, error) {
	pid, err := peer.IDFromPrivateKey(sessionKey)
	if err != nil {
		return nil, err
	}
	return &SessionTransport{
		le:            le.WithField("transport-peer", pid.String()[:8]),
		parentBus:     parentBus,
		sessionKey:    sessionKey,
		peerID:        pid,
		signalingURL:  signalingURL,
		signingEnvPfx: signingEnvPfx,
		ready:         make(chan struct{}),
	}, nil
}

// GetPeerID returns the transport's peer ID.
func (t *SessionTransport) GetPeerID() peer.ID {
	return t.peerID
}

// GetChildBus returns the child bus, or nil if not yet started.
func (t *SessionTransport) GetChildBus() bus.Bus {
	return t.childBus
}

// Ready returns a channel that is closed when the child bus and base
// controllers are started.
func (t *SessionTransport) Ready() <-chan struct{} {
	return t.ready
}

// AwaitReady blocks until the transport's child bus is created and base
// controllers are started, or until ctx is canceled.
func (t *SessionTransport) AwaitReady(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.ready:
		return nil
	}
}

// Execute creates the child bus with bifrost transport controllers and
// blocks until ctx is canceled.
func (t *SessionTransport) Execute(ctx context.Context) error {
	le := t.le

	// Create child bus with loader and resolver infrastructure.
	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	t.childBus = b

	// Bridge directives from child to parent.
	// Exclude GetPeer since the child has its own peer controller.
	bridge := bus_bridge.NewBusBridge(t.parentBus, func(di directive.Instance) (bool, error) {
		switch di.GetDirective().(type) {
		case peer.GetPeer:
			return false, nil
		}
		return true, nil
	})
	if _, err := b.AddController(ctx, bridge, nil); err != nil {
		return err
	}

	// Register peer controller with the session's private key.
	sessionPeer, err := peer.NewPeer(t.sessionKey)
	if err != nil {
		return err
	}
	peerCtrl := peer_controller.NewController(le, sessionPeer)
	if _, err := b.AddController(ctx, peerCtrl, nil); err != nil {
		return err
	}

	// Register bifrost transport factories on the child bus.
	sr.AddFactory(websocket.NewFactory(b))
	sr.AddFactory(webrtc.NewFactory(b))
	sr.AddFactory(link_solicit_controller.NewFactory())
	sr.AddFactory(dex_solicit.NewFactory(b))

	// Start solicit controller for bilateral stream matching.
	_, _, solicitRef, err := loader.WaitExecControllerRunning(
		ctx, b,
		resolver.NewLoadControllerWithConfig(&link_solicit_controller.Config{}),
		nil,
	)
	if err != nil {
		return err
	}
	defer solicitRef.Release()

	// Start signaling and WebRTC if configured.
	if t.signalingURL != "" {
		// Acquire JWT ticket for signaling WebSocket.
		ticket, err := acquireSignalTicket(ctx, t.signalingURL, t.sessionKey, t.peerID, t.signingEnvPfx)
		if err != nil {
			return err
		}

		// Build WebSocket URL from the base URL.
		wsURL := strings.Replace(t.signalingURL, "https://", "wss://", 1)
		wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
		wsURL += "/api/signal/ws?tk=" + ticket

		le.Debug("connecting to signaling")

		// Dial signaling server directly via WebSocket.
		sigClient, sigConn, sigCleanup, err := dialSignalingClient(ctx, le, wsURL, t.sessionKey)
		if err != nil {
			return err
		}
		defer sigCleanup()

		// Add signaling controller to the bus.
		sigCtrl := newWSSignalingCtrl(le, b, sigClient, sigConn, "webrtc", t.peerID)
		if _, err := b.AddController(ctx, sigCtrl, nil); err != nil {
			return err
		}

		// WebRTC transport for peer-to-peer connections.
		_, _, rtcRef, err := loader.WaitExecControllerRunning(
			ctx, b,
			resolver.NewLoadControllerWithConfig(&webrtc.Config{
				SignalingId: "webrtc",
				WebRtc: &webrtc.WebRtcConfig{
					IceServers: []*webrtc.IceServerConfig{
						{Urls: []string{"stun:stun.l.google.com:19302"}},
					},
				},
				AllPeers: true,
			}),
			nil,
		)
		if err != nil {
			return err
		}
		defer rtcRef.Release()

		le.Debug("signaling and webrtc controllers started")
	}

	// Signal ready after all controllers (including signaling) are started.
	close(t.ready)
	le.Debug("session transport started")
	<-ctx.Done()
	return ctx.Err()
}
