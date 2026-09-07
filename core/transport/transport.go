package transport

import (
	"context"
	"time"

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
	le *logrus.Entry
	// parentBus is the parent controller bus to bridge directives to.
	parentBus bus.Bus
	// bcast guards transport handles and startup lifecycle state.
	bcast broadcast.Broadcast
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
	// bridgeFilter optionally excludes directives from the parent bridge.
	bridgeFilter bus_bridge.FilterFn
	// startupTimeout bounds the readiness phase for every consumer.
	startupTimeout time.Duration
	// startupDeadlineCtx is the startup budget shared by every readiness waiter.
	startupDeadlineCtx context.Context
	// startupDeadlineCancel stops the startup budget after a terminal outcome.
	startupDeadlineCancel context.CancelFunc
	// startupDeadlineStarted prevents retries and waiters from resetting the budget.
	startupDeadlineStarted bool
	// cancel cancels the SessionTransport context after startup stop admission.
	cancel context.CancelFunc
	// startupPhase is the startup lifecycle phase; bcast guards it together
	// with startupErr and startupStage below.
	startupPhase transportStartupPhase
	// ready closes when the child bus and base controllers become ready.
	ready chan struct{}
	// startupErr is the terminal startup error, including timeout.
	startupErr error
	// startupStage records the last startup stage entered.
	startupStage string
	// startupRetryable keeps per-attempt failures private to the retrying caller.
	startupRetryable bool
}

// transportStartupPhase is one coherent startup lifecycle state for a
// SessionTransport.
type transportStartupPhase uint8

const (
	// startupPhaseIdle is the state before the first Execute attempt enters
	// its readiness phase.
	startupPhaseIdle transportStartupPhase = iota
	// startupPhaseStarting means startup controllers are still coming up.
	startupPhaseStarting
	// startupPhaseReady means every startup controller is running; terminal.
	startupPhaseReady
	// startupPhaseStopped means an admitted timeout or explicit stop owns the
	// terminal error; retries are refused.
	startupPhaseStopped
	// startupPhaseFailed records a per-attempt failure a retrying caller may
	// reset by re-entering Execute.
	startupPhaseFailed
)

// startupTerminal reports whether the phase ends startup with a stable
// outcome: ready, an admitted stop, or a failure no retry will clear.
func (p transportStartupPhase) startupTerminal() bool {
	return p == startupPhaseReady || p == startupPhaseStopped || p == startupPhaseFailed
}

// SessionTransportOption configures child-bus directive routing and startup.
type SessionTransportOption func(*SessionTransport)

const defaultSessionTransportStartupTimeout = 2 * time.Minute

// WithStartupTimeout bounds the transport startup readiness phase.
func WithStartupTimeout(timeout time.Duration) SessionTransportOption {
	return func(t *SessionTransport) {
		if timeout > 0 {
			t.startupTimeout = timeout
		}
	}
}

// WithBridgeDirectiveFilter excludes matching directives from the generic
// child-to-parent bridge while SessionTransport keeps forwarding the rest.
func WithBridgeDirectiveFilter(filter bus_bridge.FilterFn) SessionTransportOption {
	return func(t *SessionTransport) {
		t.bridgeFilter = filter
	}
}

// WithStartupRetry enables retrying startup semantics. Execute attempts leave
// transient failures to the retrying caller, while AwaitReady reports only the
// transport's admitted terminal outcome.
func WithStartupRetry() SessionTransportOption {
	return func(t *SessionTransport) {
		t.startupRetryable = true
	}
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
	opts ...SessionTransportOption,
) (*SessionTransport, error) {
	pid, err := peer.IDFromPrivateKey(sessionKey)
	if err != nil {
		return nil, err
	}
	t := &SessionTransport{
		le:             le.WithField("transport-peer", pid.String()[:8]),
		parentBus:      parentBus,
		sessionKey:     sessionKey,
		peerID:         pid,
		signalingURL:   signalingURL,
		signingEnvPfx:  signingEnvPfx,
		startupTimeout: defaultSessionTransportStartupTimeout,
		ready:          make(chan struct{}),
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

func (t *SessionTransport) GetChildBus() bus.Bus {
	var childBus bus.Bus
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		childBus = t.childBus
	})
	return childBus
}

// GetLinkedPeerIDsSnapshotWithWait returns linked peer IDs and a wait channel
// that closes when the transport link set changes.
func (t *SessionTransport) GetLinkedPeerIDsSnapshotWithWait(peerIDs []peer.ID) (map[peer.ID]struct{}, <-chan struct{}) {
	var linkController *transport_controller.Controller
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		linkController = t.linkController
	})
	if linkController == nil {
		return nil, nil
	}
	return linkController.GetLinkedPeerIDsSnapshotWithWait(peerIDs)
}

// GetLinkSnapshotsWithWait returns live link snapshots and a wait channel that
// closes when the transport link set changes.
func (t *SessionTransport) GetLinkSnapshotsWithWait() ([]transport_controller.LinkSnapshot, <-chan struct{}) {
	var linkController *transport_controller.Controller
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		linkController = t.linkController
	})
	if linkController == nil {
		return nil, nil
	}
	return linkController.GetLinkSnapshotsWithWait()
}

// Ready returns a channel that closes when the child bus and base controllers
// become ready.
func (t *SessionTransport) Ready() <-chan struct{} {
	return t.ready
}

// GetStartupStage returns the last startup stage entered by Execute.
func (t *SessionTransport) GetStartupStage() string {
	var stage string
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		stage = t.startupStage
	})
	return stage
}

func (t *SessionTransport) setStartupStage(stage string) {
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.startupStage == stage {
			return
		}
		t.startupStage = stage
		broadcast()
	})
}

func (t *SessionTransport) ensureStartupDeadline(ctx context.Context) {
	var deadlineCtx context.Context
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if t.startupDeadlineStarted {
			return
		}
		deadlineCtx, t.startupDeadlineCancel = context.WithTimeout(
			context.WithoutCancel(ctx),
			t.startupTimeout,
		)
		t.startupDeadlineCtx = deadlineCtx
		t.startupDeadlineStarted = true
	})
	if deadlineCtx == nil {
		return
	}
	go func() {
		<-deadlineCtx.Done()
		if deadlineCtx.Err() == context.DeadlineExceeded {
			_ = t.admitStartupTimeout()
		}
	}()
}

func (t *SessionTransport) cancelStartupDeadline() {
	var cancel context.CancelFunc
	t.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		cancel = t.startupDeadlineCancel
		t.startupDeadlineCancel = nil
	})
	if cancel != nil {
		cancel()
	}
}

// AwaitReady blocks until the transport's child bus and base controllers are
// started, startup fails, the startup budget expires, or ctx is canceled.
func (t *SessionTransport) AwaitReady(ctx context.Context) error {
	return t.awaitReady(ctx, nil)
}

func (t *SessionTransport) awaitReady(ctx context.Context, beforeWait func()) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	t.ensureStartupDeadline(ctx)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var (
			phase        transportStartupPhase
			startupErr   error
			startupStage string
			waitCh       <-chan struct{}
			deadlineCtx  context.Context
			deadlineCh   <-chan struct{}
		)
		t.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			phase = t.startupPhase
			startupErr = t.startupErr
			startupStage = t.startupStage
			if phase == startupPhaseStarting || phase == startupPhaseIdle {
				waitCh = getWaitCh()
				deadlineCtx = t.startupDeadlineCtx
				if deadlineCtx != nil {
					deadlineCh = deadlineCtx.Done()
				}
			}
		})
		if phase == startupPhaseReady {
			return nil
		}
		if startupErr != nil {
			if phase == startupPhaseStopped {
				return startupErr
			}
			return errors.Wrapf(startupErr, "session transport failed to start at %s", startupStage)
		}
		if beforeWait != nil {
			beforeWait()
		}

		if deadlineCh != nil {
			select {
			case <-deadlineCh:
				if deadlineCtx.Err() != context.DeadlineExceeded {
					if err := ctx.Err(); err != nil {
						return err
					}
					continue
				}
				if err := t.admitStartupTimeout(); err != nil {
					return err
				}
				continue
			default:
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadlineCh:
			if deadlineCtx == nil || deadlineCtx.Err() != context.DeadlineExceeded {
				if err := ctx.Err(); err != nil {
					return err
				}
				continue
			}
			if err := t.admitStartupTimeout(); err != nil {
				return err
			}
		case <-waitCh:
		}
	}
}

func (t *SessionTransport) admitStartupTimeout() error {
	var (
		err    error
		cancel context.CancelFunc
	)
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.startupPhase == startupPhaseReady {
			return
		}
		if t.startupErr != nil {
			err = t.startupErr
			if t.startupPhase != startupPhaseStopped {
				err = errors.Wrapf(err, "session transport failed to start at %s", t.startupStage)
			}
			return
		}
		err = errors.Errorf(
			"session transport did not become ready, stalled at %s",
			t.startupStage,
		)
		t.startupPhase = startupPhaseStopped
		t.startupErr = err
		cancel = t.cancel
		broadcast()
	})
	if cancel != nil {
		cancel()
	}
	return err
}

func (t *SessionTransport) publishStartupError(err error) bool {
	var (
		cancel    context.CancelFunc
		published bool
	)
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.startupPhase.startupTerminal() {
			return
		}
		t.startupErr = err
		t.startupPhase = startupPhaseFailed
		published = true
		cancel = t.startupDeadlineCancel
		t.startupDeadlineCancel = nil
		broadcast()
	})
	if cancel != nil {
		cancel()
	}
	return published
}

func (t *SessionTransport) publishStartupReady() {
	var cancel context.CancelFunc
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if t.startupPhase.startupTerminal() {
			return
		}
		t.startupPhase = startupPhaseReady
		close(t.ready)
		cancel = t.startupDeadlineCancel
		t.startupDeadlineCancel = nil
		broadcast()
	})
	if cancel != nil {
		cancel()
	}
}

// Execute creates the child bus with bifrost transport controllers and
// blocks until ctx is canceled.
func (t *SessionTransport) Execute(ctx context.Context) (err error) {
	// Initialize cancellation and publish startup state.
	ctx, cancel := context.WithCancel(ctx)
	t.ensureStartupDeadline(ctx)
	var stopped bool
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.cancel = cancel
		stopped = t.startupPhase == startupPhaseStopped
		if !stopped && t.startupPhase != startupPhaseReady {
			t.startupErr = nil
			t.startupStage = ""
			t.startupPhase = startupPhaseStarting
		}
		broadcast()
	})
	if stopped {
		cancel()
		return context.Canceled
	}
	defer cancel()
	defer func() {
		if errors.Is(err, context.Canceled) {
			t.cancelStartupDeadline()
		}
	}()

	le := t.le
	if !t.startupRetryable {
		defer func() {
			t.publishStartupError(err)
		}()
	} else {
		defer func() {
			if errors.Is(err, errSignalTicketUnauthorized) && t.publishStartupError(err) {
				err = nil
			}
		}()
	}

	// Create the child bus and its controller infrastructure.
	t.setStartupStage("child-bus")
	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return err
	}
	t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		t.childBus = b
		broadcast()
	})
	defer func() {
		t.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			t.childBus = nil
			t.linkController = nil
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
			t.linkController = rtcCtrl
			broadcast()
		})
	}

	t.setStartupStage("ready")
	t.publishStartupReady()
	le.Debug("session transport started")
	<-ctx.Done()
	return ctx.Err()
}
