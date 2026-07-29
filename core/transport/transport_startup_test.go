package transport

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSessionTransportStartupTimeoutCannotBeOverwrittenByReady(t *testing.T) {
	st := &SessionTransport{startupTimeout: time.Second}
	st.setStartupStage("ready")

	timeoutErr := st.admitStartupTimeout()
	if timeoutErr == nil {
		t.Fatal("expected startup timeout admission")
	}

	st.publishStartupReady()

	if st.startupReady {
		t.Fatal("startup timeout was overwritten by readiness")
	}
	err := st.AwaitReady(context.Background())
	if err == nil || !strings.Contains(err.Error(), "session transport did not become ready") {
		t.Fatalf("AwaitReady returned %v after timeout admission", err)
	}
}

func TestSessionTransportPublicationWinsInternalDeadlineCancellation(t *testing.T) {
	terminalErr := errors.New("terminal startup")
	tests := []struct {
		name      string
		publish   func(*SessionTransport)
		check     func(error) bool
		wantError string
	}{
		{
			name: "ready",
			publish: func(st *SessionTransport) {
				st.publishStartupReady()
			},
			check: func(err error) bool {
				return err == nil
			},
		},
		{
			name: "terminal error",
			publish: func(st *SessionTransport) {
				st.publishStartupError(terminalErr)
			},
			check: func(err error) bool {
				return errors.Is(err, terminalErr)
			},
			wantError: terminalErr.Error(),
		},
		{
			name: "unauthorized",
			publish: func(st *SessionTransport) {
				st.publishStartupError(&signalTicketHTTPError{statusCode: 401})
			},
			check: func(err error) bool {
				return errors.Is(err, errSignalTicketUnauthorized)
			},
			wantError: errSignalTicketUnauthorized.Error(),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			st := &SessionTransport{
				startupTimeout: time.Second,
				ready:          make(chan struct{}),
			}
			snapshot := make(chan struct{})
			release := make(chan struct{})
			var once sync.Once
			done := make(chan error, 1)
			go func() {
				done <- st.awaitReady(ctx, func() {
					once.Do(func() {
						close(snapshot)
						<-release
					})
				})
			}()

			select {
			case <-snapshot:
			case <-ctx.Done():
				t.Fatalf("AwaitReady did not capture its wait epoch: %v", ctx.Err())
			}
			tt.publish(st)
			close(release)

			select {
			case err := <-done:
				if !tt.check(err) {
					t.Fatalf("AwaitReady returned %v after %s publication, want %s", err, tt.name, tt.wantError)
				}
			case <-ctx.Done():
				t.Fatalf("AwaitReady did not observe %s publication: %v", tt.name, ctx.Err())
			}
		})
	}
}

func TestSessionTransportCancellationDoesNotAdmitStartupTimeout(t *testing.T) {
	callerCtx, callerCancel := context.WithCancel(t.Context())
	defer callerCancel()
	st := &SessionTransport{startupTimeout: time.Second}
	startupCtx, startupCancel := context.WithTimeout(t.Context(), time.Second)
	defer startupCancel()
	st.ensureStartupDeadline(startupCtx)
	st.cancelStartupDeadline()

	err := st.awaitReady(callerCtx, callerCancel)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("AwaitReady returned %v after owner cancellation, want context canceled", err)
	}
}
