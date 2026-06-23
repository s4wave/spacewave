package plugin_host_configset

import (
	"testing"

	"github.com/pkg/errors"
)

func TestIsWebRuntimeClientClosed(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "web runtime instance closed",
			err:  errors.New("WebRuntimeClientInstance closed: plugin/spacewave-core"),
			want: true,
		},
		{
			name: "runtime client normal close",
			err:  errors.New("RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: normal-close"),
			want: true,
		},
		{
			name: "runtime client normal close without error name",
			err:  errors.New("WebRuntimeClient: plugin/spacewave-core: runtime client generation 12 closed: normal-close"),
			want: true,
		},
		{
			name: "runtime client normal close wrapped",
			err:  errors.New("apply configset: RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: normal-close"),
			want: true,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "generic instance closed",
			err:  errors.New("WebRuntimeClientInstance is closed"),
			want: false,
		},
		{
			name: "runtime client runtime error",
			err:  errors.New("RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed: runtime-error"),
			want: false,
		},
		{
			name: "runtime client malformed generation",
			err:  errors.New("RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation first closed: normal-close"),
			want: false,
		},
		{
			name: "runtime client missing close reason",
			err:  errors.New("RuntimeClientClosedError: WebRuntimeClient: plugin/spacewave-core: runtime client generation 1 closed"),
			want: false,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isWebRuntimeClientClosed(test.err); got != test.want {
				t.Fatalf("isWebRuntimeClientClosed() = %v, want %v", got, test.want)
			}
		})
	}
}
