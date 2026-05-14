package transport

import (
	"context"
	"sync"

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
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// SessionTransport manages a session-scoped child bus with bifrost
// transport controllers bound to the session's peer identity.
type SessionTransport struct {
	le *logrus.Entry
	// parentBus is the parent controller bus to bridge directives to.
	parentBus bus.Bus
	mtx       sync.RWMutex
	// childBus is the session-scoped child bus.
	childBus bus.Bus
	// linkController is the active transport controller that owns link state.
	linkController *transport_controller.Controller
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
	t.mtx.RLock()
	defer t.mtx.RUnlock()
	return t.childBus
}

// GetLinkedPeerIDsSnapshotWithWait returns linked peer IDs and a wait channel
// that closes when the transport link set changes.
func (t *SessionTransport) GetLinkedPeerIDsSnapshotWithWait(peerIDs []peer.ID) (map[peer.ID]struct{}, <-chan struct{}) {
	t.mtx.RLock()
	linkController := t.linkController
	t.mtx.RUnlock()
	if linkController == nil {
		return nil, nil
	}
	return linkController.GetLinkedPeerIDsSnapshotWithWait(peerIDs)
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
	t.mtx.Lock()
	t.childBus = b
	t.mtx.Unlock()
	defer func() {
		t.mtx.Lock()
		t.childBus = nil
		t.linkController = nil
		t.mtx.Unlock()
	}()

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
	for _, factory := range sessionTransportFactories(b) {
		sr.AddFactory(factory)
	}
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

	rtcCtrl, releaseRTC, err := t.startWebRTCControllers(ctx, le, b)
	if err != nil {
		return err
	}
	if releaseRTC != nil {
		defer releaseRTC()
	}
	if rtcCtrl != nil {
		t.mtx.Lock()
		t.linkController = rtcCtrl
		t.mtx.Unlock()
	}

	// Signal ready after all controllers (including signaling) are started.
	close(t.ready)
	le.Debug("session transport started")
	<-ctx.Done()
	return ctx.Err()
}
