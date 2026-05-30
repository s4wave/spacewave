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
	readyRc   *routine.RoutineContainer

	bcast  broadcast.Broadcast
	ready  bool
	exited bool
	err    error
}

func newSessionTransportState(
	a *ProviderAccount,
	st *transport.SessionTransport,
) *sessionTransportState {
	sts := &sessionTransportState{
		transport: st,
	}
	sts.rc = routine.NewRoutineContainerWithLogger(
		a.le.WithField("routine", "session-transport"),
		routine.WithExitCb(func(err error) {
			sts.setExited(err)
			sts.readyRc.ClearContext()
			if err != nil && !errors.Is(err, context.Canceled) {
				a.le.WithError(err).Warn("session transport exited with error")
			}
			a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if a.sessionTransport == sts {
					a.sessionTransport = nil
					broadcast()
				}
			})
		}),
	)
	sts.rc.SetRoutine(st.Execute)
	sts.readyRc = routine.NewRoutineContainerWithLogger(
		a.le.WithField("routine", "session-transport-ready"),
	)
	sts.readyRc.SetRoutine(sts.watchReady)
	return sts
}

func (s *sessionTransportState) Start(ctx context.Context) {
	s.readyRc.SetContext(ctx, false)
	s.rc.SetContext(ctx, false)
}

func (s *sessionTransportState) Stop() {
	waitCh, _ := s.rc.SetRoutine(nil)
	s.readyRc.ClearContext()
	s.rc.ClearContext()
	s.setExited(context.Canceled)
	_ = s.readyRc.WaitExited(context.Background(), true, nil)
	if waitCh != nil {
		<-waitCh
	}
}

func (s *sessionTransportState) WaitStarted(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var ready bool
		var exited bool
		var exitErr error
		var ch <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ready = s.ready
			exited = s.exited
			exitErr = s.err
			ch = getWaitCh()
		})
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
			return ctx.Err()
		case <-ch:
		}
	}
}

func (s *sessionTransportState) watchReady(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.transport.Ready():
		s.setReady()
		return nil
	}
}

func (s *sessionTransportState) setReady() {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.ready || s.exited {
			return
		}
		s.ready = true
		broadcast()
	})
}

func (s *sessionTransportState) setExited(err error) {
	s.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if s.exited {
			return
		}
		s.exited = true
		s.err = err
		broadcast()
	})
}

// CreateSessionTransport creates and starts a session transport using the
// given session private key and signaling URL. If a transport is already
// running, it is stopped first.
func (a *ProviderAccount) CreateSessionTransport(
	ctx context.Context,
	sessionKey crypto.PrivKey,
	signalingURL string,
) error {
	a.transportReplaceMtx.Lock()
	a.stopSessionTransportLocked(nil)

	st, err := transport.NewSessionTransport(a.le, a.p.b, sessionKey, signalingURL, a.p.signingEnvPfx)
	if err != nil {
		a.transportReplaceMtx.Unlock()
		return errors.Wrap(err, "create session transport")
	}

	sts := newSessionTransportState(a, st)

	a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		a.sessionTransport = sts
		broadcast()
	})
	sts.Start(ctx)
	a.transportReplaceMtx.Unlock()

	if err := sts.WaitStarted(ctx); err != nil {
		a.stopSessionTransport(sts)
		return err
	}
	return nil
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
	a.transportReplaceMtx.Lock()
	defer a.transportReplaceMtx.Unlock()
	a.stopSessionTransportLocked(nil)
}

func (a *ProviderAccount) stopSessionTransport(target *sessionTransportState) {
	a.transportReplaceMtx.Lock()
	defer a.transportReplaceMtx.Unlock()
	a.stopSessionTransportLocked(target)
}

func (a *ProviderAccount) stopSessionTransportLocked(target *sessionTransportState) {
	var sts *sessionTransportState
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sts = a.sessionTransport
	})
	if sts == nil || (target != nil && sts != target) {
		return
	}
	sts.Stop()
	a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.sessionTransport == sts {
			a.sessionTransport = nil
			broadcast()
		}
	})
}
