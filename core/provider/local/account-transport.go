package provider_local

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// sessionTransportState holds a running SessionTransport.
type sessionTransportState struct {
	transport *transport.SessionTransport
	rc        *routine.RoutineContainer
}

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
	defer rel()

	return a.createSessionTransportLocked(ctx, sessionKey, signalingURL)
}

func (a *ProviderAccount) createSessionTransportLocked(ctx context.Context, sessionKey crypto.PrivKey, signalingURL string) (*sessionTransportState, error) {
	a.stopSessionTransportLocked()

	st, err := transport.NewSessionTransport(a.le, a.t.p.b, sessionKey, signalingURL, "")
	if err != nil {
		return nil, errors.Wrap(err, "create session transport")
	}

	// exitedCh signals startup failure (Execute returned before Ready).
	exitedCh := make(chan struct{}, 1)
	var exitErr error
	var sts *sessionTransportState

	rc := routine.NewRoutineContainerWithLogger(
		a.le.WithField("routine", "session-transport"),
		routine.WithExitCb(func(err error) {
			exitErr = err
			select {
			case exitedCh <- struct{}{}:
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

	// Wait for ready or startup failure.
	select {
	case <-ctx.Done():
		a.stopSessionTransportLocked()
		return nil, ctx.Err()
	case <-exitedCh:
		return nil, errors.Wrap(exitErr, "session transport failed to start")
	case <-st.Ready():
		return sts, nil
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

// lookupCloudEndpoint resolves the cloud provider API endpoint via the bus.
// Returns empty string if no cloud provider is configured (transport will
// run without WebRTC signaling).
func (a *ProviderAccount) lookupCloudEndpoint(ctx context.Context) string {
	// endpointProvider is satisfied by providers that expose a cloud API endpoint.
	type endpointProvider interface {
		GetEndpoint() string
	}
	swProv, swProvRef, err := provider.ExLookupProvider(ctx, a.t.p.b, "spacewave", true, nil)
	if err != nil || swProv == nil {
		a.le.Debug("no spacewave provider found, transport will run without signaling")
		return ""
	}
	defer swProvRef.Release()
	ep, ok := swProv.(endpointProvider)
	if !ok {
		a.le.Warn("spacewave provider does not expose endpoint")
		return ""
	}
	endpoint := ep.GetEndpoint()
	a.le.WithField("signaling-url", endpoint).Debug("resolved cloud signaling endpoint")
	return endpoint
}

// EnsureSessionTransport creates the session transport if not already running.
func (a *ProviderAccount) EnsureSessionTransport(
	ctx context.Context,
	sessionPriv crypto.PrivKey,
	relayURL string,
) error {
	rel, err := a.mtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer rel()

	st := a.GetSessionTransport()
	if st != nil {
		a.le.Debug("session transport already exists, skipping creation")
		return st.AwaitReady(ctx)
	}
	_, err = a.createSessionTransportLocked(ctx, sessionPriv, relayURL)
	return err
}

// GetOnlinePeerIDs returns the base58 peer IDs of paired devices that
// currently have an active bifrost link on the session transport.
func (a *ProviderAccount) GetOnlinePeerIDs(ctx context.Context, peerIDs []string) []string {
	st := a.GetSessionTransport()
	if st == nil {
		return nil
	}
	childBus := st.GetChildBus()
	if childBus == nil {
		return nil
	}
	localPeerID := st.GetPeerID()

	var online []string
	for _, pidStr := range peerIDs {
		remotePeerID, err := peer.IDB58Decode(pidStr)
		if err != nil {
			continue
		}
		lnk, rel, err := link.EstablishLinkWithPeerEx(ctx, childBus, localPeerID, remotePeerID, true)
		if err != nil || lnk == nil {
			continue
		}
		rel()
		online = append(online, pidStr)
	}
	return online
}
