package provider_spacewave

import (
	"context"
	"net/http"
	"time"

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

var errSessionTransportUnauthorized = errors.New("session transport unauthorized")

type sessionTransportStatusError interface {
	StatusCode() int
}

type unauthorizedSessionTransportError struct {
	err error
}

func (e *unauthorizedSessionTransportError) Error() string {
	return e.err.Error()
}

func (e *unauthorizedSessionTransportError) Unwrap() error {
	return e.err
}

func (e *unauthorizedSessionTransportError) Is(target error) bool {
	return target == errSessionTransportUnauthorized
}

func classifySessionTransportError(err error) error {
	var statusErr sessionTransportStatusError
	if errors.As(err, &statusErr) && statusErr.StatusCode() == http.StatusUnauthorized {
		return &unauthorizedSessionTransportError{err: err}
	}
	return err
}

func sessionTransportReplacementContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return sessionTransportCleanupContext(ctx)
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
		routine.WithRetry(providerBackoff),
		routine.WithExitCb(func(err error) {
			a.handleSessionTransportExit(sessionID, sts, err)
		}),
	)
	sts.rc.SetRoutine(st.Execute)
	sts.readyRc = routine.NewRoutineContainerWithLogger(
		a.le.WithField("routine", "session-transport-ready"),
	)
	sts.readyRc.SetRoutine(sts.watchReady)
	return sts
}

func (a *ProviderAccount) handleSessionTransportExit(
	sessionID string,
	sts *sessionTransportState,
	err error,
) {
	var ready bool
	sts.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ready = sts.ready
	})
	if !ready {
		if !errors.Is(err, context.Canceled) {
			a.le.WithError(err).WithField("session-id", sessionID).Warn("session transport startup attempt failed")
		}
		return
	}
	sts.setExited(err)
	sts.readyRc.ClearContext()
	a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.sessionTransports[sessionID] == sts {
			delete(a.sessionTransports, sessionID)
			broadcast()
		}
	})
}

func (s *sessionTransportState) Start(ctx context.Context) {
	s.readyRc.SetContext(ctx, false)
	s.rc.SetContext(ctx, false)
}

func (s *sessionTransportState) Stop(ctx context.Context) error {
	readyWaitCh, _ := s.readyRc.SetRoutine(nil)
	runWaitCh, _ := s.rc.SetRoutine(nil)
	s.readyRc.ClearContext()
	s.rc.ClearContext()
	s.setExited(context.Canceled)
	if readyWaitCh != nil {
		select {
		case <-readyWaitCh:
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "wait for session transport readiness routine exit")
		}
	}
	if runWaitCh != nil {
		select {
		case <-runWaitCh:
		case <-ctx.Done():
			return errors.Wrap(ctx.Err(), "wait for session transport routine exit")
		}
	}
	return nil
}

func (s *sessionTransportState) WaitStarted(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		var ready bool
		var exited bool
		var exitErr error
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ready = s.ready
			exited = s.exited
			exitErr = s.err
			waitCh = getWaitCh()
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
		case <-waitCh:
		}
	}
}

func (s *sessionTransportState) watchReady(ctx context.Context) error {
	if err := s.transport.AwaitReady(ctx); err != nil {
		s.setExited(err)
		return err
	}
	s.setReady()
	return nil
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
	cleanupCtx, cleanupCancel := sessionTransportReplacementContext(ctx)
	rel, err := a.transportReplaceMtx.Lock(cleanupCtx)
	if err != nil {
		cleanupCancel()
		return err
	}
	if err := a.stopSessionTransportLocked(cleanupCtx, sessionID, nil); err != nil {
		rel()
		cleanupCancel()
		return err
	}

	st, err := transport.NewSessionTransport(
		a.le,
		a.p.b,
		sessionKey,
		signalingURL,
		a.p.signingEnvPfx,
		transport.WithStartupRetry(),
		transport.WithBridgeDirectiveFilter(func(di directive.Instance) (bool, error) {
			_, isMount := di.GetDirective().(sobject.MountSharedObject)
			return !isMount, nil
		}),
	)
	if err != nil {
		rel()
		cleanupCancel()
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
	rel()
	cleanupCancel()

	if err := sts.WaitStarted(ctx); err != nil {
		err = classifySessionTransportError(err)
		if stopErr := a.stopSessionTransportForSession(ctx, sessionID, sts); stopErr != nil {
			return errors.Wrap(stopErr, "cleanup failed session transport startup")
		}
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
	if err := a.stopSessionTransportForSession(nil, "", nil); err != nil {
		a.le.WithError(err).Warn("failed to stop session transport")
	}
}

func (a *ProviderAccount) stopSessionTransportForSession(
	ctx context.Context,
	sessionID string,
	target *sessionTransportState,
) error {
	cleanupCtx, cleanupCancel := sessionTransportCleanupContext(ctx)
	defer cleanupCancel()
	rel, err := a.transportReplaceMtx.Lock(cleanupCtx)
	if err != nil {
		return err
	}
	defer rel()
	return a.stopSessionTransportLocked(cleanupCtx, sessionID, target)
}

func (a *ProviderAccount) stopSessionTransportLocked(
	ctx context.Context,
	sessionID string,
	target *sessionTransportState,
) error {
	var sts *sessionTransportState
	a.transportBcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		sts = a.sessionTransports[sessionID]
	})
	if sts == nil || target != nil && sts != target {
		return nil
	}
	if err := sts.Stop(ctx); err != nil {
		return err
	}
	a.transportBcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if a.sessionTransports[sessionID] == sts {
			delete(a.sessionTransports, sessionID)
			broadcast()
		}
	})
	return nil
}
