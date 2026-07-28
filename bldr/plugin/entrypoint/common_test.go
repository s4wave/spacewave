package plugin_entrypoint

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/starpc/srpc"
)

func TestHandlePluginEntrypointError(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		wantForwarded bool
	}{
		{
			name: "normal generation close ignored",
			err: errors.New(
				"RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: normal-close",
			),
			wantForwarded: false,
		},
		{
			name: "runtime error forwarded",
			err: errors.New(
				"RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: runtime-error",
			),
			wantForwarded: true,
		},
		{
			name: "malformed generation forwarded",
			err: errors.New(
				"RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation first closed: normal-close",
			),
			wantForwarded: true,
		},
		{
			name:          "instance close forwarded",
			err:           errors.New("WebRuntimeClientInstance closed: plugin/spacewave-core"),
			wantForwarded: true,
		},
		{
			name:          "fatal error forwarded",
			err:           errors.New("fatal controller error"),
			wantForwarded: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errCh := make(chan error, 1)
			handlePluginEntrypointError(errCh, test.err)

			select {
			case got := <-errCh:
				if !test.wantForwarded {
					t.Fatalf("normal lifecycle error was forwarded: %v", got)
				}
				if got != test.err {
					t.Fatalf("forwarded error %v, want %v", got, test.err)
				}
			default:
				if test.wantForwarded {
					t.Fatal("fatal error was not forwarded")
				}
			}
		})
	}
}

func TestIsExpectedPluginEntrypointError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "context canceled",
			err:  context.Canceled,
			want: true,
		},
		{
			name: "normal generation close",
			err: errors.New(
				"RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: normal-close",
			),
			want: true,
		},
		{
			name: "runtime error",
			err: errors.New(
				"RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: runtime-error",
			),
			want: false,
		},
		{
			name: "malformed generation",
			err: errors.New(
				"RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation first closed: normal-close",
			),
			want: false,
		},
		{
			name: "instance close",
			err:  errors.New("WebRuntimeClientInstance closed: plugin/spacewave-core"),
			want: false,
		},
		{
			name: "fatal error",
			err:  errors.New("fatal controller error"),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isExpectedPluginEntrypointError(test.err); got != test.want {
				t.Fatalf(
					"isExpectedPluginEntrypointError() = %v, want %v",
					got,
					test.want,
				)
			}
		})
	}
}

func TestStartInitialCapabilityRegistration(t *testing.T) {
	ctx := t.Context()

	handlerReady := make(chan func(), 1)
	completeCalled := make(chan struct{}, 1)
	released := make(chan struct{}, 1)
	resultCh := make(chan struct {
		release func()
		err     error
	}, 1)
	errCh := make(chan error, 1)

	go func() {
		release, err := startInitialCapabilityRegistration(
			ctx,
			nil,
			func(ctx context.Context, _ *srpc.Server, ready func()) error {
				handlerReady <- ready
				<-ctx.Done()
				return ctx.Err()
			},
			errCh,
			func(context.Context) (func(), error) {
				completeCalled <- struct{}{}
				return func() {
					released <- struct{}{}
				}, nil
			},
		)
		resultCh <- struct {
			release func()
			err     error
		}{release: release, err: err}
	}()

	ready := <-handlerReady
	select {
	case <-completeCalled:
		t.Fatal("initial capability registration completed before the stream handler was ready")
	default:
	}

	ready()
	result := <-resultCh
	if result.err != nil {
		t.Fatal(result.err)
	}
	select {
	case <-completeCalled:
	default:
		t.Fatal("initial capability registration did not complete after the stream handler was ready")
	}
	select {
	case <-released:
		t.Fatal("initial capability registrations were released before entrypoint shutdown")
	default:
	}

	result.release()
	select {
	case <-released:
	default:
		t.Fatal("initial capability registrations were not released at entrypoint shutdown")
	}
}

func TestStartInitialCapabilityRegistrationHandlerFailure(t *testing.T) {
	wantErr := errors.New("stream handler failed")
	completeCalled := false

	_, err := startInitialCapabilityRegistration(
		context.Background(),
		nil,
		func(context.Context, *srpc.Server, func()) error {
			return wantErr
		},
		make(chan error, 1),
		func(context.Context) (func(), error) {
			completeCalled = true
			return func() {}, nil
		},
	)
	if !errors.Is(err, wantErr) {
		t.Fatalf("startInitialCapabilityRegistration() error = %v, want %v", err, wantErr)
	}
	if completeCalled {
		t.Fatal("initial capability registration completed after the stream handler failed")
	}
}

func TestStartInitialCapabilityRegistrationCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	handlerStarted := make(chan struct{})
	resultCh := make(chan error, 1)
	completeCalled := false

	go func() {
		_, err := startInitialCapabilityRegistration(
			ctx,
			nil,
			func(ctx context.Context, _ *srpc.Server, _ func()) error {
				close(handlerStarted)
				<-ctx.Done()
				return ctx.Err()
			},
			make(chan error, 1),
			func(context.Context) (func(), error) {
				completeCalled = true
				return func() {}, nil
			},
		)
		resultCh <- err
	}()

	<-handlerStarted
	cancel()
	if err := <-resultCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("startInitialCapabilityRegistration() error = %v, want %v", err, context.Canceled)
	}
	if completeCalled {
		t.Fatal("initial capability registration completed after cancellation")
	}
}
