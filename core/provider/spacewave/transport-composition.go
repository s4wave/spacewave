package provider_spacewave

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
)

// TransportCompositionP2PState describes one Session's direct lifecycle.
type TransportCompositionP2PState uint8

const (
	// TransportCompositionP2PStateUnknown has no reported lifecycle state.
	TransportCompositionP2PStateUnknown TransportCompositionP2PState = iota
	// TransportCompositionP2PStateDisabled has direct transport disabled.
	TransportCompositionP2PStateDisabled
	// TransportCompositionP2PStateStarting is starting direct transport.
	TransportCompositionP2PStateStarting
	// TransportCompositionP2PStateNoPeers has not established a peer link.
	TransportCompositionP2PStateNoPeers
	// TransportCompositionP2PStateIdle has peer links without active demand.
	TransportCompositionP2PStateIdle
	// TransportCompositionP2PStateActive has peer links serving active demand.
	TransportCompositionP2PStateActive
	// TransportCompositionP2PStateFallbackNoPeer has lost its established peer links.
	TransportCompositionP2PStateFallbackNoPeer
	// TransportCompositionP2PStateError could not keep direct transport running.
	TransportCompositionP2PStateError
)

// TransportCompositionSnapshot is one Session's direct transport projection.
type TransportCompositionSnapshot struct {
	// DirectP2PEnabled records the session policy.
	DirectP2PEnabled bool
	// P2PState reports the current direct lifecycle.
	P2PState TransportCompositionP2PState
	// ActivePeerCount counts active transport links.
	ActivePeerCount uint32
	// LastError describes the most recent transport failure.
	LastError string
	// Unauthorized reports that the cloud rejected the Session credential.
	Unauthorized bool
}

// transportCompositionLinkSource exposes transport-owned link state and changes.
type transportCompositionLinkSource interface {
	// GetLinkSnapshotsWithWait returns links and every channel that can change them.
	GetLinkSnapshotsWithWait() ([]transport_controller.LinkSnapshot, []<-chan struct{})
	// Err returns the failure that ended the transport, or nil.
	Err() error
}

type transportCompositionConfig struct {
	sessionID    string
	sessionKey   crypto.PrivKey
	signalingURL string
	enabled      bool
}

type transportCompositionSession struct {
	mtx           sync.Mutex
	config        *transportCompositionConfig
	directRunning bool
	linkCancel    context.CancelFunc
	// bcast guards the link-goroutine lifetime and every projection field below.
	bcast         broadcast.Broadcast
	linkRunning   bool
	snapshot      TransportCompositionSnapshot
	generation    uint64
	hadPeers      bool
	activeDemands uint64
	closing       bool
}

type transportCompositionOwner struct {
	account *ProviderAccount

	mtx      sync.Mutex
	sessions map[string]*transportCompositionSession

	// Hooks keep the state machine independently testable. Production hooks bind
	// transportCompositionOwner to the account's SessionTransport and P2P
	// lifecycle maps.
	startDirect    func(context.Context, string, crypto.PrivKey, string) (transportCompositionLinkSource, error)
	stopDirect     func(string)
	transportState func(string) (bool, <-chan struct{})
}

// init binds the owner to its account and production hooks. Idempotent: the
// first caller constructs the shared state; later calls return it unchanged.
func (o *transportCompositionOwner) init(account *ProviderAccount) {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	if o.sessions != nil {
		return
	}
	o.account = account
	o.sessions = make(map[string]*transportCompositionSession)
	o.startDirect = func(ctx context.Context, sessionID string, sessionKey crypto.PrivKey, signalingURL string) (transportCompositionLinkSource, error) {
		if err := account.createSessionTransportForSession(ctx, sessionID, sessionKey, signalingURL); err != nil {
			return nil, err
		}
		st := account.getSessionTransportForSession(sessionID)
		if st == nil {
			if err := account.stopSessionTransportForSession(ctx, sessionID, nil); err != nil {
				return nil, errors.Wrap(err, "stop missing session transport")
			}
			return nil, errors.New("session transport missing after startup")
		}
		if err := account.startP2PSyncForSession(ctx, sessionID, st); err != nil {
			account.stopP2PSyncForSession(sessionID)
			if stopErr := account.stopSessionTransportForSession(ctx, sessionID, nil); stopErr != nil {
				return nil, errors.Wrap(stopErr, "stop session transport after P2P startup failure")
			}
			return nil, err
		}
		return st, nil
	}
	o.stopDirect = func(sessionID string) {
		account.stopP2PSyncForSession(sessionID)
		if err := account.stopSessionTransportForSession(nil, sessionID, nil); err != nil {
			account.le.WithError(err).Warn("failed to stop session transport composition")
		}
	}
	o.transportState = func(sessionID string) (bool, <-chan struct{}) {
		return account.getTransportSnapshotWithWaitForSession(sessionID)
	}
}

// newTransportCompositionSession constructs the initial direct transport projection.
func newTransportCompositionSession() *transportCompositionSession {
	return &transportCompositionSession{
		snapshot: TransportCompositionSnapshot{
			DirectP2PEnabled: true,
			P2PState:         TransportCompositionP2PStateNoPeers,
		},
	}
}

// sessionForConfigure returns the shared session state, creating it on first configuration.
func (o *transportCompositionOwner) sessionForConfigure(sessionID string) *transportCompositionSession {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	state := o.sessions[sessionID]
	if state == nil {
		state = newTransportCompositionSession()
		o.sessions[sessionID] = state
	}
	return state
}

// findSession reads the current session state under the owner lock.
func (o *transportCompositionOwner) findSession(sessionID string) *transportCompositionSession {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	return o.sessions[sessionID]
}

// ConfigureSessionTransport reconciles one mounted Session's direct policy.
// Startup work is bound to ctx; the running link watcher outlives ctx and ends
// only when the composition stops.
func (a *ProviderAccount) ConfigureSessionTransport(ctx context.Context, sessionID string, sessionKey crypto.PrivKey, signalingURL string, enabled bool) error {
	o := &a.transportComposition
	o.init(a)
	return o.configure(ctx, &transportCompositionConfig{sessionID: sessionID, sessionKey: sessionKey, signalingURL: signalingURL, enabled: enabled})
}

// SetSessionDirectP2PEnabled reconciles one mounted Session after persistence.
func (a *ProviderAccount) SetSessionDirectP2PEnabled(ctx context.Context, sessionID string, enabled bool) error {
	o := &a.transportComposition
	o.init(a)
	return o.setEnabled(ctx, sessionID, enabled)
}

// StopSessionTransportComposition stops direct mechanics owned by sessionID.
func (a *ProviderAccount) StopSessionTransportComposition(sessionID string) {
	o := &a.transportComposition
	o.init(a)
	o.stop(sessionID)
}

// GetTransportCompositionSnapshotWithWait returns one Session's state.
func (a *ProviderAccount) GetTransportCompositionSnapshotWithWait(sessionID string) (TransportCompositionSnapshot, <-chan struct{}) {
	o := &a.transportComposition
	o.init(a)
	return o.snapshotWithWait(sessionID)
}

// directDemandStarted records direct work for the mounted session.
func (a *ProviderAccount) directDemandStarted(sessionID string) {
	o := &a.transportComposition
	o.init(a)
	o.demandStarted(sessionID)
}

// directDemandFinished releases direct work for the mounted session.
func (a *ProviderAccount) directDemandFinished(sessionID string) {
	o := &a.transportComposition
	o.init(a)
	o.demandFinished(sessionID)
}

// configure serializes configuration against session shutdown.
func (o *transportCompositionOwner) configure(ctx context.Context, config *transportCompositionConfig) error {
	state := o.sessionForConfigure(config.sessionID)
	state.mtx.Lock()
	defer state.mtx.Unlock()
	o.mtx.Lock()
	current := o.sessions[config.sessionID]
	o.mtx.Unlock()
	if current != state || state.closing {
		return errors.New("session transport composition is stopping")
	}
	return o.configureLocked(ctx, state, config)
}

// configureLocked reconciles direct transport and its watcher under the session lock.
func (o *transportCompositionOwner) configureLocked(ctx context.Context, state *transportCompositionSession, config *transportCompositionConfig) error {
	if state.config != nil && state.config.enabled == config.enabled &&
		state.snapshotState() != TransportCompositionP2PStateError {
		state.config = config
		return nil
	}

	o.stopLocked(state, false)
	state.config = config
	if !config.enabled {
		state.setSnapshot(TransportCompositionSnapshot{DirectP2PEnabled: false, P2PState: TransportCompositionP2PStateDisabled})
		return nil
	}

	state.setSnapshot(TransportCompositionSnapshot{DirectP2PEnabled: true, P2PState: TransportCompositionP2PStateStarting})
	linkSource, err := o.startDirect(ctx, config.sessionID, config.sessionKey, config.signalingURL)
	if err != nil {
		state.setSnapshot(TransportCompositionSnapshot{
			DirectP2PEnabled: true,
			P2PState:         TransportCompositionP2PStateError,
			LastError:        err.Error(),
			Unauthorized:     sessionTransportUnauthorized(err),
		})
		return err
	}

	state.directRunning = true
	// The watcher belongs to the composition lifetime, not to the caller's
	// context: a later SetSessionDirectP2PEnabled restart must not inherit a
	// caller whose lifecycle already ended.
	linkCtx, linkCancel := context.WithCancel(context.WithoutCancel(ctx))
	state.linkCancel = linkCancel
	generation := state.nextGeneration()
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.linkRunning = true
		broadcast()
	})
	go func() {
		defer func() {
			state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				state.linkRunning = false
				broadcast()
			})
		}()
		o.watchLinks(linkCtx, config.sessionID, state, generation, linkSource)
	}()
	return nil
}

// setEnabled updates the mounted session policy under its lifecycle lock.
func (o *transportCompositionOwner) setEnabled(ctx context.Context, sessionID string, enabled bool) error {
	state := o.findSession(sessionID)
	if state == nil {
		return nil
	}
	state.mtx.Lock()
	defer state.mtx.Unlock()
	if state.closing {
		return errors.New("session transport composition is stopping")
	}
	if state.config == nil {
		return nil
	}
	config := *state.config
	config.enabled = enabled
	return o.configureLocked(ctx, state, &config)
}

// stop stops the session composition before removing its shared state.
func (o *transportCompositionOwner) stop(sessionID string) {
	state := o.findSession(sessionID)
	if state == nil {
		return
	}
	state.mtx.Lock()
	if state.closing {
		state.mtx.Unlock()
		return
	}
	state.closing = true
	if state.config == nil {
		state.mtx.Unlock()
		o.removeSession(sessionID, state)
		return
	}
	enabled := state.config.enabled
	o.stopLocked(state, true)
	state.setSnapshot(TransportCompositionSnapshot{DirectP2PEnabled: enabled, P2PState: TransportCompositionP2PStateDisabled})
	state.mtx.Unlock()
	o.removeSession(sessionID, state)
}

// removeSession removes only the session state being stopped.
func (o *transportCompositionOwner) removeSession(sessionID string, target *transportCompositionSession) {
	o.mtx.Lock()
	if o.sessions[sessionID] == target {
		delete(o.sessions, sessionID)
	}
	o.mtx.Unlock()
}

// awaitLinkExit waits for the running link watcher to publish its exit.
func (o *transportCompositionOwner) awaitLinkExit(state *transportCompositionSession) {
	for {
		var waitCh <-chan struct{}
		exited := false
		state.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if state.linkRunning {
				waitCh = getWaitCh()
				return
			}
			exited = true
		})
		if exited {
			return
		}
		<-waitCh
	}
}

// stopLocked cancels and joins the watcher before stopping direct transport.
func (o *transportCompositionOwner) stopLocked(state *transportCompositionSession, clearConfig bool) {
	state.nextGeneration()
	if state.linkCancel != nil {
		state.linkCancel()
		state.linkCancel = nil
	}
	o.awaitLinkExit(state)
	if state.directRunning {
		o.stopDirect(state.config.sessionID)
		state.directRunning = false
	}
	if clearConfig {
		state.config = nil
	}
}

// watchLinks projects transport-owned snapshots until cancellation or transport exit.
func (o *transportCompositionOwner) watchLinks(ctx context.Context, sessionID string, state *transportCompositionSession, generation uint64, source transportCompositionLinkSource) {
	for {
		running, transportWaitCh := o.transportState(sessionID)
		if !running {
			o.setTransportExited(state, generation, source.Err())
			return
		}
		links, linkWaitChs := source.GetLinkSnapshotsWithWait()
		o.setLinks(state, generation, len(links))
		if len(linkWaitChs) == 0 && transportWaitCh == nil {
			return
		}
		if err := broadcast.WaitAny(ctx, append(linkWaitChs, transportWaitCh)...); err != nil {
			return
		}
	}
}

// setLinks publishes links only for the current watcher generation.
func (o *transportCompositionOwner) setLinks(state *transportCompositionSession, generation uint64, count int) {
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if generation != state.generation {
			return
		}
		state.snapshot.ActivePeerCount = uint32(max(count, 0)) //nolint:gosec // count is the non-negative length of the active peer set.
		switch {
		case count > 0:
			state.hadPeers = true
			p2p := TransportCompositionP2PStateIdle
			if state.activeDemands != 0 {
				p2p = TransportCompositionP2PStateActive
			}
			state.snapshot.P2PState = p2p
		case state.hadPeers:
			state.snapshot.P2PState = TransportCompositionP2PStateFallbackNoPeer
		default:
			state.snapshot.P2PState = TransportCompositionP2PStateNoPeers
		}
		state.snapshot.LastError = ""
		broadcast()
	})
}

// setTransportExited records unexpected exit only for the current watcher
// generation. err is the transport failure, or nil when it stopped cleanly.
func (o *transportCompositionOwner) setTransportExited(state *transportCompositionSession, generation uint64, err error) {
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if generation != state.generation {
			return
		}
		state.snapshot.ActivePeerCount = 0
		state.snapshot.P2PState = TransportCompositionP2PStateError
		state.snapshot.LastError = "session transport stopped"
		if err != nil {
			state.snapshot.LastError = err.Error()
		}
		state.snapshot.Unauthorized = sessionTransportUnauthorized(err)
		broadcast()
	})
}

// demandStarted counts active demand while direct peer links exist.
func (o *transportCompositionOwner) demandStarted(sessionID string) {
	state := o.findSession(sessionID)
	if state == nil {
		return
	}
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !state.snapshot.DirectP2PEnabled || state.snapshot.ActivePeerCount == 0 {
			return
		}
		state.activeDemands++
		state.snapshot.P2PState = TransportCompositionP2PStateActive
		broadcast()
	})
}

// demandFinished returns linked transport to idle when its final demand ends.
func (o *transportCompositionOwner) demandFinished(sessionID string) {
	state := o.findSession(sessionID)
	if state == nil {
		return
	}
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if state.activeDemands == 0 {
			return
		}
		state.activeDemands--
		if state.activeDemands == 0 && state.snapshot.ActivePeerCount > 0 {
			state.snapshot.P2PState = TransportCompositionP2PStateIdle
		}
		broadcast()
	})
}

// nextGeneration invalidates earlier watcher updates and resets demand history.
func (state *transportCompositionSession) nextGeneration() uint64 {
	var generation uint64
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.generation++
		state.hadPeers = false
		state.activeDemands = 0
		generation = state.generation
		broadcast()
	})
	return generation
}

// setSnapshot publishes replacement lifecycle state and resets demand history.
func (state *transportCompositionSession) setSnapshot(snapshot TransportCompositionSnapshot) {
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.snapshot = snapshot
		state.hadPeers = false
		state.activeDemands = 0
		broadcast()
	})
}

// snapshotState reads the projected direct lifecycle under its broadcast lock.
func (state *transportCompositionSession) snapshotState() TransportCompositionP2PState {
	var p2pState TransportCompositionP2PState
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p2pState = state.snapshot.P2PState
	})
	return p2pState
}

// snapshotWithWait returns the current projection and its matching change channel.
func (o *transportCompositionOwner) snapshotWithWait(sessionID string) (TransportCompositionSnapshot, <-chan struct{}) {
	state := o.findSession(sessionID)
	if state == nil {
		return TransportCompositionSnapshot{
			DirectP2PEnabled: true,
			P2PState:         TransportCompositionP2PStateNoPeers,
		}, nil
	}
	var snapshot TransportCompositionSnapshot
	var waitCh <-chan struct{}
	state.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		snapshot = state.snapshot
		waitCh = getWaitCh()
	})
	return snapshot, waitCh
}

var _ transportCompositionLinkSource = (*transport.SessionTransport)(nil)
