package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// sessionTransportState holds a running SessionTransport.
type sessionTransportState struct {
	transport *transport.SessionTransport
	rc        *routine.RoutineContainer
}

type cloudRelayEndpoint struct {
	url              string
	signingEnvPrefix string
}

var errSessionTransportReplaced = errors.New("session transport replaced before ready")

// CreateSessionTransport creates and starts a session transport using the
// given session private key and signaling URL. If a transport is already
// running, it is stopped first.
//
// The transport runs via a RoutineContainer. On post-Ready failures, the
// exit callback clears sessionTransport and broadcasts.
func (a *ProviderAccount) CreateSessionTransport(ctx context.Context, sessionKey crypto.PrivKey, signalingURL string) error {
	_, err := a.createSessionTransport(ctx, sessionKey, signalingURL)
	return err
}

func (a *ProviderAccount) createSessionTransport(ctx context.Context, sessionKey crypto.PrivKey, signalingURL string) (*sessionTransportState, error) {
	rel, err := a.mtx.Lock(ctx)
	if err != nil {
		return nil, err
	}
	sts, exitedCh, err := a.startSessionTransportLocked(ctx, sessionKey, signalingURL, "")
	rel()
	if err != nil {
		return nil, err
	}
	if err := a.waitSessionTransportReady(ctx, sts, exitedCh); err != nil {
		return nil, err
	}
	return sts, nil
}

func (a *ProviderAccount) startSessionTransportLocked(ctx context.Context, sessionKey crypto.PrivKey, signalingURL string, signingEnvPrefix string) (*sessionTransportState, <-chan error, error) {
	a.stopSessionTransportLocked()

	st, err := transport.NewSessionTransport(a.le, a.t.p.b, sessionKey, signalingURL, signingEnvPrefix)
	if err != nil {
		return nil, nil, errors.Wrap(err, "create session transport")
	}

	// exitedCh signals startup failure (Execute returned before Ready).
	exitedCh := make(chan error, 1)
	var sts *sessionTransportState

	rc := routine.NewRoutineContainerWithLogger(
		a.le.WithField("routine", "session-transport"),
		routine.WithExitCb(func(err error) {
			select {
			case exitedCh <- err:
			default:
			}
			if err != nil && !errors.Is(err, context.Canceled) {
				a.le.WithError(err).Warn("session transport exited with error")
				// If a pairing is active, surface the error as SIGNALING_FAILED.
				a.SetPairingSignalingFailed(err.Error())
			}
			a.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
				if a.sessionTransport == sts {
					a.sessionTransport = nil
					bcast()
				}
			})
		}),
	)
	sts = &sessionTransportState{
		transport: st,
		rc:        rc,
	}

	rc.SetRoutine(st.Execute)
	rc.SetContext(ctx, false)

	a.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		a.sessionTransport = sts
		bcast()
	})

	return sts, exitedCh, nil
}

func (a *ProviderAccount) waitSessionTransportReady(ctx context.Context, sts *sessionTransportState, exitedCh <-chan error) error {
	select {
	case <-ctx.Done():
		a.stopSessionTransportState(sts)
		return ctx.Err()
	case err := <-exitedCh:
		return errors.Wrap(err, "session transport failed to start")
	case <-sts.transport.Ready():
		return nil
	}
}

func (a *ProviderAccount) waitExistingSessionTransportReady(ctx context.Context, sts *sessionTransportState) error {
	for {
		var waitCh <-chan struct{}
		var current bool
		a.transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			current = a.sessionTransport == sts
		})
		if !current {
			return errSessionTransportReplaced
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-sts.transport.Ready():
			return nil
		case <-waitCh:
		}
	}
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
	rel, err := a.mtx.Lock(context.Background())
	if err != nil {
		return
	}
	defer rel()

	a.stopSessionTransportLocked()
}

func (a *ProviderAccount) stopSessionTransportState(sts *sessionTransportState) {
	rel, err := a.mtx.Lock(context.Background())
	if err != nil {
		return
	}
	defer rel()

	a.stopSessionTransportStateLocked(sts)
}

func (a *ProviderAccount) stopSessionTransportLocked() {
	var sts *sessionTransportState
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sts = a.sessionTransport
	})
	if sts == nil {
		return
	}
	a.stopSessionTransportStateLocked(sts)
}

func (a *ProviderAccount) stopSessionTransportStateLocked(sts *sessionTransportState) {
	if sts == nil {
		return
	}
	sts.rc.ClearContext()
	_ = sts.rc.WaitExited(context.Background(), true, nil)
	// Clear explicitly: WaitExited may return before the exit callback runs.
	a.transportBcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if a.sessionTransport == sts {
			a.sessionTransport = nil
			bcast()
		}
	})
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

func (a *ProviderAccount) ensureSessionTransport(
	ctx context.Context,
	sessionPriv crypto.PrivKey,
	relayURL string,
	signingEnvPrefix string,
) (*sessionTransportState, bool, error) {
	for {
		rel, err := a.mtx.Lock(ctx)
		if err != nil {
			return nil, false, err
		}

		var sts *sessionTransportState
		a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			sts = a.sessionTransport
		})
		if sts != nil {
			rel()
			a.le.Debug("session transport already exists, skipping creation")
			err := a.waitExistingSessionTransportReady(ctx, sts)
			if errors.Is(err, errSessionTransportReplaced) {
				continue
			}
			return sts, false, err
		}
		sts, exitedCh, err := a.startSessionTransportLocked(ctx, sessionPriv, relayURL, signingEnvPrefix)
		rel()
		if err != nil {
			return nil, false, err
		}
		return sts, true, a.waitSessionTransportReady(ctx, sts, exitedCh)
	}
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
