package provider_spacewave

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
)

// sessionTransportState holds a running SessionTransport for one Session.
type sessionTransportState struct {
	sessionID string
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
	sessionID string,
	st *transport.SessionTransport,
) *sessionTransportState {
	sts := &sessionTransportState{
		sessionID: sessionID,
		transport: st,
	}
	sts.rc = routine.NewRoutineContainerWithLogger(
		a.le.WithField("routine", "session-transport"),
		routine.WithExitCb(func(err error) {
			sts.setExited(err)
			sts.readyRc.ClearContext()
			if err != nil && !errors.Is(err, context.Canceled) {
				a.le.WithError(err).WithField("session-id", sessionID).Warn("session transport exited with error")
			}
			a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				if a.sessionTransports[sessionID] == sts {
					delete(a.sessionTransports, sessionID)
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

// CreateSessionTransport creates and starts the legacy default session transport.
func (a *ProviderAccount) CreateSessionTransport(
	ctx context.Context,
	sessionKey crypto.PrivKey,
	signalingURL string,
) error {
	return a.createSessionTransportForSession(ctx, "", sessionKey, signalingURL)
}

func (a *ProviderAccount) createSessionTransportForSession(
	ctx context.Context,
	sessionID string,
	sessionKey crypto.PrivKey,
	signalingURL string,
) error {
	a.transportReplaceMtx.Lock()
	a.stopSessionTransportLocked(sessionID, nil)

	st, err := transport.NewSessionTransport(
		a.le,
		a.p.b,
		sessionKey,
		signalingURL,
		a.p.signingEnvPfx,
		transport.WithBridgeDirectiveFilter(func(di directive.Instance) (bool, error) {
			_, isMount := di.GetDirective().(sobject.MountSharedObject)
			return !isMount, nil
		}),
	)
	if err != nil {
		a.transportReplaceMtx.Unlock()
		return errors.Wrap(err, "create session transport")
	}

	sts := newSessionTransportState(a, sessionID, st)
	a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.sessionTransports == nil {
			a.sessionTransports = make(map[string]*sessionTransportState)
		}
		a.sessionTransports[sessionID] = sts
		broadcast()
	})
	sts.Start(ctx)
	a.transportReplaceMtx.Unlock()

	if err := sts.WaitStarted(ctx); err != nil {
		a.stopSessionTransportForSession(sessionID, sts)
		return err
	}
	return nil
}

// GetSessionTransport returns the legacy default session transport, or nil.
func (a *ProviderAccount) GetSessionTransport() *transport.SessionTransport {
	return a.getSessionTransportForSession("")
}

func (a *ProviderAccount) getSessionTransportForSession(sessionID string) *transport.SessionTransport {
	var st *transport.SessionTransport
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if state := a.sessionTransports[sessionID]; state != nil {
			st = state.transport
		}
	})
	return st
}

// getSessionChildBusForSession returns the live child bus, or nil when the
// Session transport is absent, disabled, or already exited.
func (a *ProviderAccount) getSessionChildBusForSession(sessionID string) bus.Bus {
	var state *sessionTransportState
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = a.sessionTransports[sessionID]
	})
	if state == nil {
		return nil
	}
	var exited bool
	state.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		exited = state.exited
	})
	if exited || state.transport == nil {
		return nil
	}
	return state.transport.GetChildBus()
}

// getSessionBusForSession returns the live child bus and falls back to the
// account parent bus when direct transport is absent or has exited.
func (a *ProviderAccount) getSessionBusForSession(sessionID string) bus.Bus {
	if child := a.getSessionChildBusForSession(sessionID); child != nil {
		return child
	}
	if a.p != nil {
		return a.p.b
	}
	return nil
}

// GetTransportBroadcast returns the transport state broadcast.
func (a *ProviderAccount) GetTransportBroadcast() *broadcast.Broadcast {
	return &a.transportBcast
}

// GetTransportSnapshotWithWait returns the legacy default transport state.
func (a *ProviderAccount) GetTransportSnapshotWithWait() (bool, <-chan struct{}) {
	return a.getTransportSnapshotWithWaitForSession("")
}

func (a *ProviderAccount) getTransportSnapshotWithWaitForSession(sessionID string) (bool, <-chan struct{}) {
	var running bool
	var ch <-chan struct{}
	a.transportBcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		ch = getWaitCh()
		running = a.sessionTransports[sessionID] != nil
	})
	return running, ch
}

// StopSessionTransport stops the legacy default session transport.
func (a *ProviderAccount) StopSessionTransport() {
	a.stopSessionTransportForSession("", nil)
}

func (a *ProviderAccount) stopSessionTransportForSession(sessionID string, target *sessionTransportState) {
	a.transportReplaceMtx.Lock()
	defer a.transportReplaceMtx.Unlock()
	a.stopSessionTransportLocked(sessionID, target)
}

func (a *ProviderAccount) stopSessionTransportLocked(sessionID string, target *sessionTransportState) {
	var sts *sessionTransportState
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sts = a.sessionTransports[sessionID]
	})
	if sts == nil || (target != nil && sts != target) {
		return
	}
	sts.Stop()
	a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.sessionTransports[sessionID] == sts {
			delete(a.sessionTransports, sessionID)
			broadcast()
		}
	})
}
