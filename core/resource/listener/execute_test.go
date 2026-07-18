//go:build !js

package resource_listener

import (
	"testing"

	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
)

func TestHandoffBlocksActiveListener(t *testing.T) {
	const socketPath = "/tmp/spacewave.sock"
	tests := []struct {
		name    string
		handoff yield_policy.HandoffState
		want    bool
	}{
		{
			name: "inactive",
		},
		{
			name: "matching socket",
			handoff: yield_policy.HandoffState{
				Active:     true,
				SocketPath: socketPath,
			},
			want: true,
		},
		{
			name: "unscoped process handoff",
			handoff: yield_policy.HandoffState{
				Active: true,
			},
			want: true,
		},
		{
			name: "different socket",
			handoff: yield_policy.HandoffState{
				Active:     true,
				SocketPath: "/tmp/other.sock",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handoffBlocksListener(tt.handoff); got != tt.want {
				t.Fatalf("handoffBlocksListener() = %v, want %v", got, tt.want)
			}
		})
	}
}
