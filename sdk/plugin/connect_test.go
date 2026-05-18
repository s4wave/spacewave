package s4wave_plugin

import (
	"errors"
	"testing"
)

func TestIsTransientPluginResourceClientInitError(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "pre init context canceled",
			err:  errors.New("receive resource client init: context canceled"),
			want: true,
		},
		{
			name: "pre init stream reset",
			err:  errors.New("receive resource client init: stream reset"),
			want: true,
		},
		{
			name: "pre init eof",
			err:  errors.New("receive resource client init: EOF"),
			want: true,
		},
		{
			name: "stream start context canceled",
			err:  errors.New("start resource client stream: context canceled"),
			want: false,
		},
		{
			name: "post init server error",
			err:  errors.New("resource client: server rejected request"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := isTransientPluginResourceClientInitError(tc.err)
			if got != tc.want {
				t.Fatalf("isTransientPluginResourceClientInitError() = %v, want %v", got, tc.want)
			}
		})
	}
}
