package plugin_entrypoint

import (
	"context"
	"errors"
	"testing"
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
