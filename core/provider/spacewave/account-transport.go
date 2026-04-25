package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
)

// sessionTransportState holds a running SessionTransport.
type sessionTransportState struct {
	transport *transport.SessionTransport
	rc        *routine.RoutineContainer
}

// CreateSessionTransport creates and starts a session transport using the
// given session private key and signaling URL. If a transport is already
// running, it is stopped first.
func (a *ProviderAccount) CreateSessionTransport(
	ctx context.Context,
	sessionKey crypto.PrivKey,
	signalingURL string,
) error {
	a.StopSessionTransport()

	st, err := transport.NewSessionTransport(a.le, a.p.b, sessionKey, signalingURL, a.p.signingEnvPfx)
	if err != nil {
		return errors.Wrap(err, "create session transport")
	}

	exitedCh := make(chan struct{}, 1)
	var exitErr error

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
			}
			a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				a.sessionTransport = nil
				broadcast()
			})
		}),
	)

	rc.SetRoutine(st.Execute)
	rc.SetContext(ctx, false)

	a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.sessionTransport = &sessionTransportState{
			transport: st,
			rc:        rc,
		}
		broadcast()
	})

	select {
	case <-ctx.Done():
		a.StopSessionTransport()
		return ctx.Err()
	case <-exitedCh:
		return errors.Wrap(exitErr, "session transport failed to start")
	case <-st.Ready():
		return nil
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
	var sts *sessionTransportState
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sts = a.sessionTransport
	})
	if sts == nil {
		return
	}
	sts.rc.ClearContext()
	_ = sts.rc.WaitExited(context.Background(), true, nil)
	a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.sessionTransport == sts {
			a.sessionTransport = nil
			broadcast()
		}
	})
}
