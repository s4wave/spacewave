package transport

import (
	"context"
	"maps"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	link_solicit_controller "github.com/s4wave/spacewave/net/link/solicit/controller"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/s4wave/spacewave/net/signaling"
	stream_api_accept "github.com/s4wave/spacewave/net/stream/api/accept"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	transport_webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
	transport_websocket "github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

// SessionTransport manages a session-scoped child bus with bifrost
// transport controllers bound to the session's peer identity.
type SessionTransport struct {
	// le records transport and startup activity.
	le *logrus.Entry
	// parentBus is the parent controller bus to bridge directives to.
	parentBus bus.Bus
	// bcast guards transport handles and startup lifecycle state.
	bcast broadcast.Broadcast
	// childBus is the session-scoped child bus.
	childBus bus.Bus
	// lifecycleCtx owns controllers added after transport startup.
	lifecycleCtx context.Context
	// linkControllers owns the links exposed by each active transport.
	linkControllers []*transport_controller.Controller
	// startLocalTransport optionally attaches a process-local transport.
	startLocalTransport LocalTransportFunc
	// sessionKey is the session's Ed25519 private key.
	sessionKey bifrost_crypto.PrivKey
	// peerID is the peer ID derived from the session key.
	peerID peer.ID
	// signalingURL is the cloud API base URL (e.g. "https://alpha.spacewave.app").
	signalingURL string
	// signingEnvPfx is the request-signing environment prefix.
	signingEnvPfx string
	// bridgeFilter optionally excludes directives from the parent bridge.
	bridgeFilter bus_bridge.FilterFn
	// cancel cancels the running Execute when the transport fails.
	cancel context.CancelFunc
	// phase is the lifecycle phase; bcast guards it together with err and
	// stage below.
	phase transportPhase
	// ready closes when the child bus and local controllers become ready.
	ready chan struct{}
	// err is why the transport failed.
	err error
	// stage records the last startup stage entered.
	stage string
	// retryable keeps per-attempt failures private to the retrying caller.
	retryable bool
}

// transportPhase is one lifecycle state of a SessionTransport.
type transportPhase uint8

const (
	// The zero phase is the state before the first Execute attempt.
	_ transportPhase = iota
	// phaseStarting means the local controllers are still coming up.
	phaseStarting
	// phaseReady means the child bus and local controllers are running.
	phaseReady
	// phaseFailed means the transport cannot run again; err says why.
	phaseFailed
)

// SessionTransportOption configures child-bus directive routing and startup.
type SessionTransportOption func(*SessionTransport)

// LocalTransportFunc starts a process-local transport on the session bus b
// under peerID. It returns the controller owning the transport's links, or nil
// when it exposes none, and a release func that detaches it.
type LocalTransportFunc func(ctx context.Context, le *logrus.Entry, b bus.Bus, peerID peer.ID) (*transport_controller.Controller, func(), error)

// WithLocalTransport attaches a process-local transport before readiness.
func WithLocalTransport(start LocalTransportFunc) SessionTransportOption {
	return func(t *SessionTransport) {
		t.startLocalTransport = start
	}
}

// WithBridgeDirectiveFilter excludes matching directives from the generic
// child-to-parent bridge while SessionTransport keeps forwarding the rest.
func WithBridgeDirectiveFilter(filter bus_bridge.FilterFn) SessionTransportOption {
	return func(t *SessionTransport) {
		t.bridgeFilter = filter
	}
}

// WithStartupRetry enables retrying startup semantics. A failed Execute
// attempt stays private to the retrying caller, and Execute returns nil once
// the transport fails so the caller stops retrying.
func WithStartupRetry() SessionTransportOption {
	return func(t *SessionTransport) {
		t.retryable = true
	}
}

// NewSessionTransport constructs a new session-scoped transport.
//
// The child bus is created in Execute. The sessionKey is the session's
// Ed25519 private key used as the transport peer identity.
// A nil parentBus runs independently without parent routing or discovery.
//
// signalingURL is the cloud API base URL for the SignalingDO endpoint.
// If empty, WebRTC and signaling controllers are not started.
func NewSessionTransport(
	le *logrus.Entry,
	parentBus bus.Bus,
	sessionKey bifrost_crypto.PrivKey,
	signalingURL string,
	signingEnvPfx string,
	opts ...SessionTransportOption,
) (*SessionTransport, error) {
	pid, err := peer.IDFromPrivateKey(sessionKey)
	if err != nil {
		return nil, err
	}
	t := &SessionTransport{
		le:            le.WithField("transport-peer", pid.String()[:8]),
		parentBus:     parentBus,
		sessionKey:    sessionKey,
		peerID:        pid,
		signalingURL:  signalingURL,
		signingEnvPfx: signingEnvPfx,
		ready:         make(chan struct{}),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t, nil
}

// GetPeerID returns the transport's peer ID.
func (t *SessionTransport) GetPeerID() peer.ID {
	return t.peerID
}

// GetChildBus returns the active session bus, or nil outside Execute.
func (t *SessionTransport) GetChildBus() bus.Bus {
	var childBus bus.Bus
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		childBus = t.childBus
	})
	return childBus
}

// GetLinkedPeerIDsSnapshotWithWait returns linked peer IDs and their change channels.
// Each controller retains its link state; callers wait on all returned channels.
func (t *SessionTransport) GetLinkedPeerIDsSnapshotWithWait(peerIDs []peer.ID) (map[peer.ID]struct{}, []<-chan struct{}) {
	// Snapshot controller membership together with its notification channel.
	var controllers []*transport_controller.Controller
	var waitChs []<-chan struct{}
	t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		controllers = slices.Clone(t.linkControllers)
		waitChs = []<-chan struct{}{getWaitCh()}
	})

	// Read current links directly from each transport owner.
	peers := make(map[peer.ID]struct{})
	for _, controller := range controllers {
		linked, waitCh := controller.GetLinkedPeerIDsSnapshotWithWait(peerIDs)
		maps.Copy(peers, linked)
		waitChs = append(waitChs, waitCh)
	}
	return peers, waitChs
}

// GetLinkSnapshotsWithWait returns current links and all channels that can change them.
func (t *SessionTransport) GetLinkSnapshotsWithWait() ([]transport_controller.LinkSnapshot, []<-chan struct{}) {
	// Snapshot controller membership together with its notification channel.
	var controllers []*transport_controller.Controller
	var waitChs []<-chan struct{}
	t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		controllers = slices.Clone(t.linkControllers)
		waitChs = []<-chan struct{}{getWaitCh()}
	})

	// Preserve individual links when more than one transport reaches the same peer.
	var links []transport_controller.LinkSnapshot
	for _, controller := range controllers {
		current, waitCh := controller.GetLinkSnapshotsWithWait()
		links = append(links, current...)
		waitChs = append(waitChs, waitCh)
	}
	return links, waitChs
}

// Ready returns a channel that closes when the child bus and local
// controllers become ready. Signaling connects in the background and does not
// delay readiness.
func (t *SessionTransport) Ready() <-chan struct{} {
	return t.ready
}

// Err returns why the transport failed, or nil while it can still run.
func (t *SessionTransport) Err() error {
	var err error
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		err = t.err
	})
	return err
}

// GetStartupStage returns the last startup stage entered by Execute.
func (t *SessionTransport) GetStartupStage() string {
	var stage string
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		stage = t.stage
	})
	return stage
}

// setStartupStage records the startup stage Execute entered.
func (t *SessionTransport) setStartupStage(stage string) {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.stage == stage {
			return
		}
		t.stage = stage
		broadcast()
	})
}

// AwaitReady blocks until the child bus and local controllers are running,
// the transport fails, or ctx is canceled.
func (t *SessionTransport) AwaitReady(ctx context.Context) error {
	for {
		var (
			phase  transportPhase
			err    error
			stage  string
			waitCh <-chan struct{}
		)
		t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			phase, err, stage = t.phase, t.err, t.stage
			waitCh = getWaitCh()
		})
		switch phase {
		case phaseReady:
			return nil
		case phaseFailed:
			return errors.Wrapf(err, "session transport failed at %s", stage)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-waitCh:
		}
	}
}

// fail ends the transport with err and cancels the running Execute. Only the
// first failure is kept.
func (t *SessionTransport) fail(err error) {
	var cancel context.CancelFunc
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.phase == phaseFailed {
			return
		}
		t.phase = phaseFailed
		t.err = err
		cancel = t.cancel
		broadcast()
	})
	if cancel != nil {
		cancel()
	}
}

// failStartup fails a transport that has not become ready.
func (t *SessionTransport) failStartup(err error) {
	var ready bool
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ready = t.phase == phaseReady
	})
	if !ready {
		t.fail(err)
	}
}

// publishReady marks startup complete unless the transport already failed.
func (t *SessionTransport) publishReady() {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.phase != phaseStarting {
			return
		}
		t.phase = phaseReady
		close(t.ready)
		broadcast()
	})
}

// Execute creates the child bus with bifrost transport controllers and
// blocks until ctx is canceled or the transport fails.
func (t *SessionTransport) Execute(ctx context.Context) (err error) {
	// Register cancellation and enter startup unless the transport failed.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var failed error
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.phase == phaseFailed {
			failed = t.err
			return
		}
		t.cancel = cancel
		if t.phase != phaseReady {
			t.phase = phaseStarting
			t.stage = ""
		}
		broadcast()
	})
	if failed != nil {
		return t.exitFailed(failed)
	}

	// Report a failure instead of the cancellation it caused.
	defer func() {
		if failed := t.Err(); failed != nil {
			err = t.exitFailed(failed)
			return
		}
		if !t.retryable && err != nil {
			t.failStartup(err)
		}
	}()

	le := t.le

	// Create the child bus and its controller infrastructure.
	t.setStartupStage("child-bus")
	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	defer func() {
		cancel()
		closeErr := b.Close()
		if err == nil {
			err = closeErr
		}
	}()
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.childBus = b
		t.lifecycleCtx = ctx
		broadcast()
	})
	defer func() {
		t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			t.childBus = nil
			t.lifecycleCtx = nil
			t.linkControllers = nil
			broadcast()
		})
	}()

	t.setStartupStage("bridge")

	// Bridge directives from child to parent.
	bridge := bus_bridge.NewBusBridge(t.parentBus, func(di directive.Instance) (bool, error) {
		if t.bridgeFilter != nil {
			process, err := t.bridgeFilter(di)
			if err != nil || !process {
				return process, err
			}
		}
		switch d := di.GetDirective().(type) {
		case peer.GetPeer, link.EstablishLinkWithPeer, link_solicit.SolicitProtocol,
			signaling.SignalPeer, signaling.HandleSignalPeer:
			return false, nil
		case resolver.LoadControllerWithConfig:
			switch d.GetLoadControllerConfig().(type) {
			case *stream_api_accept.Config, *dex_solicit.Config, *link_solicit_controller.Config,
				*transport_webrtc.Config, *transport_websocket.Config:
				return false, nil
			}
		case loader.ExecController:
			switch d.GetExecControllerConfig().(type) {
			case *stream_api_accept.Config, *dex_solicit.Config, *link_solicit_controller.Config,
				*transport_webrtc.Config, *transport_websocket.Config:
				return false, nil
			}
		}
		return true, nil
	})
	if _, err := b.AddController(ctx, bridge, nil); err != nil {
		return err
	}

	t.setStartupStage("peer-controller")

	// Register peer controller with the session's private key.
	sessionPeer, err := peer.NewPeer(t.sessionKey)
	if err != nil {
		return err
	}
	peerCtrl := peer_controller.NewController(le, sessionPeer)
	if _, err := b.AddController(ctx, peerCtrl, nil); err != nil {
		return err
	}

	t.setStartupStage("factories")

	// Register bifrost transport factories on the child bus.
	for _, factory := range sessionTransportFactories(b) {
		sr.AddFactory(factory)
	}
	sr.AddFactory(link_solicit_controller.NewFactory())
	sr.AddFactory(dex_solicit.NewFactory(b))
	sr.AddFactory(stream_api_accept.NewFactory(b))

	t.setStartupStage("solicit-controller")

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

	// Attach process-local packet routes before announcing transport readiness.
	if t.startLocalTransport != nil {
		t.setStartupStage("local-transport")
		localCtrl, releaseLocal, err := t.startLocalTransport(ctx, le, b, t.peerID)
		if err != nil {
			return err
		}
		defer releaseLocal()
		if localCtrl != nil {
			t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				t.linkControllers = append(t.linkControllers, localCtrl)
				broadcast()
			})
		}
	}

	t.setStartupStage("webrtc-controllers")
	rtcCtrl, releaseRTC, err := t.startWebRTCControllers(ctx, le, b)
	if err != nil {
		return err
	}
	if releaseRTC != nil {
		defer releaseRTC()
	}
	if rtcCtrl != nil {
		t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			t.linkControllers = append(t.linkControllers, rtcCtrl)
			broadcast()
		})
	}

	releaseLookup, err := t.publishSessionBus(b)
	if err != nil {
		return err
	}
	defer releaseLookup()

	t.setStartupStage("ready")
	t.publishReady()
	le.Debug("session transport started")
	<-ctx.Done()
	return ctx.Err()
}

// exitFailed returns the Execute result for a failed transport: nil for a
// retrying caller, which then stops, and the failure otherwise.
func (t *SessionTransport) exitFailed(err error) error {
	if t.retryable {
		return nil
	}
	return err
}
