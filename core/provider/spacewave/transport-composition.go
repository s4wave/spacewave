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
	TransportCompositionP2PStateUnknown TransportCompositionP2PState = iota
	TransportCompositionP2PStateDisabled
	TransportCompositionP2PStateStarting
	TransportCompositionP2PStateNoPeers
	TransportCompositionP2PStateIdle
	TransportCompositionP2PStateActive
	TransportCompositionP2PStateFallbackNoPeer
	TransportCompositionP2PStateError
)

// TransportCompositionSnapshot is one Session's direct transport projection.
type TransportCompositionSnapshot struct {
	DirectP2PEnabled bool
	P2PState         TransportCompositionP2PState
	ActivePeerCount  uint32
	LastError        string
}

type transportCompositionLinkSource interface {
	GetLinkSnapshotsWithWait() ([]transport_controller.LinkSnapshot, <-chan struct{})
}

type transportCompositionConfig struct {
	ctx          context.Context
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
	linkWG        sync.WaitGroup
	closing       bool

	bcast         broadcast.Broadcast
	snapshot      TransportCompositionSnapshot
	generation    uint64
	hadPeers      bool
	activeDemands uint64
}

type transportCompositionOwner struct {
	initOnce sync.Once
	account  *ProviderAccount

	mtx      sync.Mutex
	sessions map[string]*transportCompositionSession

	// Hooks keep the state machine independently testable. Production hooks bind
	// the owner to the account's SessionTransport and P2P lifecycle maps.
	startDirect    func(context.Context, string, crypto.PrivKey, string) (transportCompositionLinkSource, error)
	stopDirect     func(string)
	transportState func(string) (bool, <-chan struct{})
}

func (o *transportCompositionOwner) init(account *ProviderAccount) {
	o.initOnce.Do(func() {
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
	})
}

func newTransportCompositionSession() *transportCompositionSession {
	return &transportCompositionSession{
		snapshot: TransportCompositionSnapshot{
			DirectP2PEnabled: true,
			P2PState:         TransportCompositionP2PStateNoPeers,
		},
	}
}

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

func (o *transportCompositionOwner) findSession(sessionID string) *transportCompositionSession {
	o.mtx.Lock()
	defer o.mtx.Unlock()
	return o.sessions[sessionID]
}

// ConfigureSessionTransport reconciles one mounted Session's direct policy.
func (a *ProviderAccount) ConfigureSessionTransport(ctx context.Context, sessionID string, sessionKey crypto.PrivKey, signalingURL string, enabled bool) error {
	o := &a.transportComposition
	o.init(a)
	return o.configure(&transportCompositionConfig{ctx: ctx, sessionID: sessionID, sessionKey: sessionKey, signalingURL: signalingURL, enabled: enabled})
}

// SetSessionDirectP2PEnabled reconciles one mounted Session after persistence.
func (a *ProviderAccount) SetSessionDirectP2PEnabled(sessionID string, enabled bool) error {
	o := &a.transportComposition
	o.init(a)
	return o.setEnabled(sessionID, enabled)
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

func (a *ProviderAccount) directDemandStarted(sessionID string) {
	o := &a.transportComposition
	o.init(a)
	o.demandStarted(sessionID)
}

func (a *ProviderAccount) directDemandFinished(sessionID string) {
	o := &a.transportComposition
	o.init(a)
	o.demandFinished(sessionID)
}

func (o *transportCompositionOwner) configure(config *transportCompositionConfig) error {
	state := o.sessionForConfigure(config.sessionID)
	state.mtx.Lock()
	defer state.mtx.Unlock()
	o.mtx.Lock()
	current := o.sessions[config.sessionID]
	o.mtx.Unlock()
	if current != state || state.closing {
		return errors.New("session transport composition is stopping")
	}
	return o.configureLocked(state, config)
}

func (o *transportCompositionOwner) configureLocked(state *transportCompositionSession, config *transportCompositionConfig) error {
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
	linkSource, err := o.startDirect(config.ctx, config.sessionID, config.sessionKey, config.signalingURL)
	if err != nil {
		state.setSnapshot(TransportCompositionSnapshot{DirectP2PEnabled: true, P2PState: TransportCompositionP2PStateError, LastError: err.Error()})
		return err
	}

	state.directRunning = true
	linkCtx, linkCancel := context.WithCancel(config.ctx)
	state.linkCancel = linkCancel
	generation := state.nextGeneration()
	state.linkWG.Go(func() { o.watchLinks(linkCtx, config.sessionID, state, generation, linkSource) })
	return nil
}

func (o *transportCompositionOwner) setEnabled(sessionID string, enabled bool) error {
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
	return o.configureLocked(state, &config)
}
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

func (o *transportCompositionOwner) removeSession(sessionID string, target *transportCompositionSession) {
	o.mtx.Lock()
	if o.sessions[sessionID] == target {
		delete(o.sessions, sessionID)
	}
	o.mtx.Unlock()
}

func (o *transportCompositionOwner) stopLocked(state *transportCompositionSession, clearConfig bool) {
	state.nextGeneration()
	if state.linkCancel != nil {
		state.linkCancel()
		state.linkCancel = nil
	}
	state.linkWG.Wait()
	if state.directRunning {
		o.stopDirect(state.config.sessionID)
		state.directRunning = false
	}
	if clearConfig {
		state.config = nil
	}
}

func (o *transportCompositionOwner) watchLinks(ctx context.Context, sessionID string, state *transportCompositionSession, generation uint64, source transportCompositionLinkSource) {
	for {
		running, transportWaitCh := o.transportState(sessionID)
		if !running {
			o.setTransportExited(state, generation)
			return
		}
		links, linkWaitCh := source.GetLinkSnapshotsWithWait()
		o.setLinks(state, generation, len(links))
		if linkWaitCh == nil && transportWaitCh == nil {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-linkWaitCh:
		case <-transportWaitCh:
		}
	}
}

func (o *transportCompositionOwner) setLinks(state *transportCompositionSession, generation uint64, count int) {
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if generation != state.generation {
			return
		}
		state.snapshot.ActivePeerCount = uint32(max(count, 0))
		if count > 0 {
			state.hadPeers = true
			if state.activeDemands != 0 {
				state.snapshot.P2PState = TransportCompositionP2PStateActive
			} else {
				state.snapshot.P2PState = TransportCompositionP2PStateIdle
			}
		} else if state.hadPeers {
			state.snapshot.P2PState = TransportCompositionP2PStateFallbackNoPeer
		} else {
			state.snapshot.P2PState = TransportCompositionP2PStateNoPeers
		}
		state.snapshot.LastError = ""
		broadcast()
	})
}

func (o *transportCompositionOwner) setTransportExited(state *transportCompositionSession, generation uint64) {
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if generation != state.generation {
			return
		}
		state.snapshot.ActivePeerCount = 0
		state.snapshot.P2PState = TransportCompositionP2PStateError
		state.snapshot.LastError = "session transport stopped"
		broadcast()
	})
}

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

func (state *transportCompositionSession) setSnapshot(snapshot TransportCompositionSnapshot) {
	state.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		state.snapshot = snapshot
		state.hadPeers = false
		state.activeDemands = 0
		broadcast()
	})
}

func (state *transportCompositionSession) snapshotState() TransportCompositionP2PState {
	var p2pState TransportCompositionP2PState
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p2pState = state.snapshot.P2PState
	})
	return p2pState
}

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

var _ transportCompositionLinkSource = ((*transport.SessionTransport)(nil))
