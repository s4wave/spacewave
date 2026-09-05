package provider_local

import (
	"context"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// sessionTransportConfig identifies the immutable configuration of a running
// session transport.
type sessionTransportConfig struct {
	peerID           peer.ID
	signalingURL     string
	signingEnvPrefix string
}

func (c sessionTransportConfig) matches(peerID peer.ID, signalingURL, signingEnvPrefix string) bool {
	return c.peerID == peerID &&
		c.signalingURL == signalingURL &&
		c.signingEnvPrefix == signingEnvPrefix
}

// sessionTransportState holds a running SessionTransport and its readiness
// lifecycle.
type sessionTransportState struct {
	transport *transport.SessionTransport
	rc        *routine.RoutineContainer
	config    sessionTransportConfig

	bcast    broadcast.Broadcast
	ready    bool
	exited   bool
	replaced bool
	err      error
}

func (s *sessionTransportState) setReady() bool {
	committed := false
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.ready || s.exited || s.replaced {
			return
		}
		s.ready = true
		committed = true
		broadcast()
	})
	return committed
}

func (s *sessionTransportState) setReplaced() {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.replaced {
			return
		}
		s.replaced = true
		broadcast()
	})
}

func (s *sessionTransportState) setExited(err error) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.exited {
			if s.err == nil || errors.Is(s.err, context.Canceled) && !errors.Is(err, context.Canceled) {
				s.err = err
				broadcast()
			}
			return
		}
		s.exited = true
		s.err = err
		broadcast()
	})
}

type cloudRelayEndpoint struct {
	url              string
	signingEnvPrefix string
}

var (
	errSessionTransportReplaced   = errors.New("session transport replaced before ready")
	errSessionTransportSuperseded = errors.New("session transport request superseded by newer configuration")
)

// sessionTransportReplacementContext lets mandatory old-transport cleanup
// outlive caller cancellation without shortening a caller-supplied deadline.
func sessionTransportReplacementContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		return sessionTransportCleanupContext(nil)
	}
	replacementCtx := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok {
		if time.Until(deadline) <= 0 {
			return sessionTransportCleanupContext(ctx)
		}
		return context.WithDeadline(replacementCtx, deadline)
	}
	return context.WithCancel(replacementCtx)
}

func sessionTransportCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) > 0 {
		return context.WithDeadline(cleanupCtx, deadline)
	}
	timeout := time.Duration(providerBackoff.GetExponential().GetMaxInterval()) * time.Millisecond
	return context.WithTimeout(cleanupCtx, timeout)
}

// CreateSessionTransport creates and starts a session transport using the
// given session private key and signaling URL. If a transport is already
// running, it is stopped first.
//
// The transport runs via a retrying RoutineContainer. Startup attempts remain
// owned by the same transport until readiness or a terminal stop.
func (a *ProviderAccount) CreateSessionTransport(ctx context.Context, sessionKey crypto.PrivKey, signalingURL string) error {
	_, err := a.createSessionTransport(ctx, sessionKey, signalingURL)
	return err
}

func (a *ProviderAccount) createSessionTransport(ctx context.Context, sessionKey crypto.PrivKey, signalingURL string) (*sessionTransportState, error) {
	cleanupCtx, cleanupCancel := sessionTransportReplacementContext(ctx)
	defer cleanupCancel()
	rel, err := a.mtx.Lock(cleanupCtx)
	if err != nil {
		return nil, err
	}
	ownerCtx := a.lifecycleCtx
	if ownerCtx == nil {
		ownerCtx = ctx
	}
	sts, err := a.startSessionTransportLocked(ownerCtx, cleanupCtx, sessionKey, signalingURL, "")
	rel()
	if err != nil {
		return nil, err
	}
	if err := a.waitSessionTransportReady(ctx, sts); err != nil {
		return nil, err
	}
	return sts, nil
}

func (a *ProviderAccount) waitExistingSessionTransportReady(ctx context.Context, sts *sessionTransportState) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var (
			providerWaitCh <-chan struct{}
			stateWaitCh    <-chan struct{}
			current        bool
			ready          bool
			replaced       bool
			exited         bool
			exitErr        error
		)
		a.transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			providerWaitCh = getWaitCh()
			current = a.sessionTransport == sts
		})
		sts.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			stateWaitCh = getWaitCh()
			ready = sts.ready
			replaced = sts.replaced
			exited = sts.exited
			exitErr = sts.err
		})
		if replaced {
			return errSessionTransportSuperseded
		}
		if !current {
			return errSessionTransportReplaced
		}
		if ready {
			return nil
		}
		if exited {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if exitErr == nil {
				exitErr = errors.New("session transport exited before ready")
			}
			return errors.Wrap(exitErr, "session transport failed to start")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-providerWaitCh:
		case <-stateWaitCh:
		}
	}
}

func (a *ProviderAccount) startSessionTransportLocked(
	ctx context.Context,
	cleanupCtx context.Context,
	sessionKey crypto.PrivKey,
	signalingURL string,
	signingEnvPrefix string,
) (*sessionTransportState, error) {
	sessionPeerID, err := peer.IDFromPrivateKey(sessionKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive session peer ID")
	}

	if err := a.stopSessionTransportForReplacementLocked(cleanupCtx); err != nil {
		return nil, err
	}

	st, err := transport.NewSessionTransport(
		a.le,
		a.t.p.b,
		sessionKey,
		signalingURL,
		signingEnvPrefix,
		transport.WithStartupRetry(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "create session transport")
	}

	var sts *sessionTransportState

	rc := routine.NewRoutineContainerWithLogger(
		a.le.WithField("routine", "session-transport"),
		routine.WithRetry(providerBackoff),
		routine.WithExitCb(func(err error) {
			var ready bool
			sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				ready = sts.ready
			})
			if !ready {
				// Pre-ready cancellation is cleaned up by the startup waiter.
				return
			}
			a.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
				if a.sessionTransport == sts {
					a.sessionTransport = nil
					bcast()
				}
			})
			sts.setExited(err)
		}),
	)
	sts = &sessionTransportState{
		transport: st,
		rc:        rc,
		config: sessionTransportConfig{
			peerID:           sessionPeerID,
			signalingURL:     signalingURL,
			signingEnvPrefix: signingEnvPrefix,
		},
	}

	rc.SetRoutine(st.Execute)
	rc.SetContext(ctx, false)

	a.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		a.sessionTransport = sts
		bcast()
	})
	return sts, nil
}

func (a *ProviderAccount) waitSessionTransportReady(ctx context.Context, sts *sessionTransportState) error {
	cleanup := func(err error) error {
		return a.cleanupSessionTransportReadyError(ctx, sts, err)
	}
	if err := ctx.Err(); err != nil {
		return cleanup(err)
	}
	waitCtx, waitCancel := context.WithCancel(ctx)
	defer waitCancel()
	readyErr := make(chan error, 1)
	go func() {
		readyErr <- sts.transport.AwaitReady(waitCtx)
	}()

	for {
		var (
			providerWaitCh <-chan struct{}
			stateWaitCh    <-chan struct{}
			current        bool
			ready          bool
			replaced       bool
			exited         bool
			exitErr        error
		)
		a.transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			providerWaitCh = getWaitCh()
			current = a.sessionTransport == sts
		})
		sts.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			stateWaitCh = getWaitCh()
			ready = sts.ready
			replaced = sts.replaced
			exited = sts.exited
			exitErr = sts.err
		})
		if replaced {
			return errSessionTransportSuperseded
		}
		if !current {
			return errSessionTransportReplaced
		}
		if ready {
			return nil
		}
		if exited {
			if exitErr == nil {
				exitErr = errors.New("session transport exited before ready")
			}
			return errors.Wrap(exitErr, "session transport failed to start")
		}

		select {
		case <-ctx.Done():
			return cleanup(ctx.Err())
		case err := <-readyErr:
			if err == nil {
				if sts.setReady() {
					return nil
				}
				continue
			}
			return classifySessionTransportReadyError(ctx, sts, err, cleanup)
		case <-providerWaitCh:
		case <-stateWaitCh:
		}
	}
}

func (a *ProviderAccount) cleanupSessionTransportReadyError(ctx context.Context, sts *sessionTransportState, err error) error {
	cleanupCtx, cleanupCancel := sessionTransportCleanupContext(ctx)
	defer cleanupCancel()
	rel, lockErr := a.mtx.Lock(cleanupCtx)
	if lockErr != nil {
		return lockErr
	}
	defer rel()

	var current, replaced bool
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		current = a.sessionTransport == sts
	})
	sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		replaced = sts.replaced
	})
	if replaced {
		return errSessionTransportSuperseded
	}
	if !current {
		return errSessionTransportReplaced
	}

	if stopErr := a.stopSessionTransportStateLocked(cleanupCtx, sts); stopErr != nil {
		return stopErr
	}
	sts.setExited(err)
	if !errors.Is(err, context.Canceled) {
		a.SetPairingSignalingFailed(err.Error())
	}
	return err
}

func classifySessionTransportReadyError(ctx context.Context, sts *sessionTransportState, err error, cleanup func(error) error) error {
	var replaced bool
	sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		replaced = sts.replaced
	})
	if ctxErr := ctx.Err(); ctxErr != nil {
		return cleanup(ctxErr)
	}
	if replaced {
		return errSessionTransportSuperseded
	}
	return cleanup(err)
}

// GetSessionTransport returns the running session transport, or nil.
func (a *ProviderAccount) GetSessionTransport() *transport.SessionTransport {
	var st *transport.SessionTransport
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if a.sessionTransport != nil {
			st = a.sessionTransport.transport
		}
	})
	return st
}

// GetTransportBroadcast returns the transport state broadcast.
func (a *ProviderAccount) GetTransportBroadcast() *broadcast.Broadcast {
	return &a.transportBcast
}

// GetTransportSnapshotWithWait returns whether transport is running and its wait channel.
func (a *ProviderAccount) GetTransportSnapshotWithWait() (bool, <-chan struct{}) {
	var running bool
	var ch <-chan struct{}
	a.transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		running = a.sessionTransport != nil
	})
	return running, ch
}

// StopSessionTransport stops the running session transport if any.
func (a *ProviderAccount) StopSessionTransport() {
	cleanupCtx, cleanupCancel := sessionTransportReplacementContext(nil)
	defer cleanupCancel()
	rel, err := a.mtx.Lock(cleanupCtx)
	if err != nil {
		a.le.WithError(err).Warn("failed to lock session transport for stop")
		return
	}
	defer rel()

	if err := a.stopSessionTransportLocked(cleanupCtx); err != nil {
		a.le.WithError(err).Warn("failed to stop session transport")
	}
}

func (a *ProviderAccount) stopSessionTransportState(sts *sessionTransportState) {
	cleanupCtx, cleanupCancel := sessionTransportReplacementContext(nil)
	defer cleanupCancel()
	rel, err := a.mtx.Lock(cleanupCtx)
	if err != nil {
		a.le.WithError(err).Warn("failed to lock session transport state for stop")
		return
	}
	defer rel()

	if err := a.stopSessionTransportStateLocked(cleanupCtx, sts); err != nil {
		a.le.WithError(err).Warn("failed to stop session transport state")
	}
}

func (a *ProviderAccount) stopSessionTransportLocked(ctx context.Context) error {
	var sts *sessionTransportState
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sts = a.sessionTransport
	})
	return a.stopSessionTransportStateLocked(ctx, sts)
}

func (a *ProviderAccount) stopSessionTransportForReplacementLocked(ctx context.Context) error {
	var sts *sessionTransportState
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sts = a.sessionTransport
	})
	if sts == nil {
		return nil
	}
	sts.setReplaced()
	if err := a.stopSessionTransportStateLocked(ctx, sts); err != nil {
		return errors.Wrap(err, "stop replaced session transport")
	}
	return nil
}

func (a *ProviderAccount) stopSessionTransportStateLocked(ctx context.Context, sts *sessionTransportState) error {
	if sts == nil {
		return nil
	}
	waitCh, _ := sts.rc.SetRoutine(nil)
	sts.rc.ClearContext()
	if waitCh != nil {
		select {
		case <-waitCh:
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "wait for session transport routine exit")
		}
	}
	a.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.sessionTransport == sts {
			a.sessionTransport = nil
			bcast()
		}
	})
	return nil
}

// fallbackSignalingEndpoint returns the configured trusted cloud signaling
// endpoint for sessions whose cloud relay lookup found nothing. Standalone
// local sessions have no linked cloud account, so the persisted provider
// signaling URL is their only rendezvous. Empty keeps them without signaling.
func (a *ProviderAccount) fallbackSignalingEndpoint() cloudRelayEndpoint {
	return cloudRelayEndpoint{
		url:              a.t.p.signalingURL,
		signingEnvPrefix: a.t.p.signalingEnvPrefix,
	}
}

// lookupCloudRelayEndpoint resolves the cloud relay endpoint and signing
// environment via the configured Spacewave Cloud provider. Empty fields keep
// local accounts usable without cloud signaling.
func (a *ProviderAccount) lookupCloudRelayEndpoint(ctx context.Context) cloudRelayEndpoint {
	type relayProvider interface {
		GetEndpoint() string
		GetSigningEnvPrefix() string
	}
	swProv, swProvRef, err := provider.ExLookupProvider(ctx, a.t.p.b, "spacewave", true, nil)
	if err != nil || swProv == nil {
		a.le.Debug("no spacewave provider found, transport will run without signaling")
		return cloudRelayEndpoint{}
	}
	defer swProvRef.Release()
	rp, ok := swProv.(relayProvider)
	if !ok {
		a.le.Warn("spacewave provider does not expose relay endpoint")
		return cloudRelayEndpoint{}
	}
	relay := cloudRelayEndpoint{
		url:              rp.GetEndpoint(),
		signingEnvPrefix: rp.GetSigningEnvPrefix(),
	}
	a.le.WithField("signaling-url", relay.url).WithField("signing-env-prefix", relay.signingEnvPrefix).Debug("resolved cloud signaling endpoint")
	return relay
}

// EnsureSessionTransport creates the session transport if not already running.
func (a *ProviderAccount) EnsureSessionTransport(
	ctx context.Context,
	sessionPriv crypto.PrivKey,
	relayURL string,
) error {
	_, _, err := a.ensureSessionTransport(ctx, sessionPriv, relayURL, "")
	return err
}

// EnsureConfiguredSessionTransport follows the session mount's transport and
// uses the provider's trusted signaling endpoint only when it must create one.
// It never replaces a mounted transport with an RPC-owned transport.
func (a *ProviderAccount) EnsureConfiguredSessionTransport(
	ctx context.Context,
	sessionPriv crypto.PrivKey,
) error {
	relay := a.fallbackSignalingEndpoint()
	ownerCtx := a.lifecycleCtx
	if ownerCtx == nil {
		ownerCtx = ctx
	}
	_, _, err := a.ensureSessionTransportWithOwner(
		ctx,
		ownerCtx,
		sessionPriv,
		relay.url,
		relay.signingEnvPrefix,
		false,
	)
	return err
}

func (a *ProviderAccount) ensureSessionTransport(
	ctx context.Context,
	sessionPriv crypto.PrivKey,
	relayURL string,
	signingEnvPrefix string,
) (*sessionTransportState, bool, error) {
	return a.ensureSessionTransportWithReplacement(ctx, sessionPriv, relayURL, signingEnvPrefix, true)
}

func (a *ProviderAccount) ensureSessionTransportWithReplacement(
	ctx context.Context,
	sessionPriv crypto.PrivKey,
	relayURL string,
	signingEnvPrefix string,
	replaceMismatched bool,
) (*sessionTransportState, bool, error) {
	return a.ensureSessionTransportWithOwner(
		ctx,
		ctx,
		sessionPriv,
		relayURL,
		signingEnvPrefix,
		replaceMismatched,
	)
}

func (a *ProviderAccount) ensureSessionTransportWithOwner(
	ctx context.Context,
	ownerCtx context.Context,
	sessionPriv crypto.PrivKey,
	relayURL string,
	signingEnvPrefix string,
	replaceMismatched bool,
) (*sessionTransportState, bool, error) {
	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return nil, false, errors.Wrap(err, "derive session peer ID")
	}

	for {
		cleanupCtx, cleanupCancel := sessionTransportReplacementContext(ctx)
		rel, err := a.mtx.Lock(cleanupCtx)
		if err != nil {
			cleanupCancel()
			return nil, false, err
		}

		var sts *sessionTransportState
		a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			sts = a.sessionTransport
		})
		if sts != nil && (!replaceMismatched || sts.config.matches(sessionPeerID, relayURL, signingEnvPrefix)) {
			rel()
			cleanupCancel()
			a.le.Debug("session transport already exists, skipping creation")
			err := a.waitExistingSessionTransportReady(ctx, sts)
			if errors.Is(err, errSessionTransportReplaced) ||
				!replaceMismatched && errors.Is(err, errSessionTransportSuperseded) {
				continue
			}
			return sts, false, err
		}

		if sts != nil {
			a.le.Debug("replacing session transport with requested configuration")
		}
		sts, err = a.startSessionTransportLocked(ownerCtx, cleanupCtx, sessionPriv, relayURL, signingEnvPrefix)
		rel()
		cleanupCancel()
		if err != nil {
			return nil, false, err
		}
		err = a.waitSessionTransportReady(ctx, sts)
		if !replaceMismatched && errors.Is(err, errSessionTransportSuperseded) {
			continue
		}
		return sts, true, err
	}
}

// ensureSessionTransportWithoutReplacement follows an installed transport
// instead of replacing it, so lifecycle startup cannot displace explicit work.
func (a *ProviderAccount) ensureSessionTransportWithoutReplacement(
	ctx context.Context,
	sessionPriv crypto.PrivKey,
	relayURL string,
	signingEnvPrefix string,
) (*sessionTransportState, bool, error) {
	return a.ensureSessionTransportWithReplacement(ctx, sessionPriv, relayURL, signingEnvPrefix, false)
}

// GetOnlinePeerIDsWithWait returns the base58 peer IDs of paired devices that
// currently have an active bifrost link and change channels for transport and
// link state.
func (a *ProviderAccount) GetOnlinePeerIDsWithWait(peerIDs []string) ([]string, []<-chan struct{}) {
	_, transportCh := a.GetTransportSnapshotWithWait()
	st := a.GetSessionTransport()
	if st == nil {
		return nil, []<-chan struct{}{transportCh}
	}

	decoded := make([]peer.ID, 0, len(peerIDs))
	peerIDStrings := make(map[peer.ID]string, len(peerIDs))
	for _, pidStr := range peerIDs {
		remotePeerID, err := peer.IDB58Decode(pidStr)
		if err != nil {
			continue
		}
		decoded = append(decoded, remotePeerID)
		peerIDStrings[remotePeerID] = pidStr
	}
	linked, linkCh := st.GetLinkedPeerIDsSnapshotWithWait(decoded)
	online := make([]string, 0, len(linked))
	for _, peerID := range decoded {
		if _, ok := linked[peerID]; ok {
			online = append(online, peerIDStrings[peerID])
		}
	}
	return online, []<-chan struct{}{transportCh, linkCh}
}
