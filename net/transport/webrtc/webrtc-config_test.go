package webrtc_test

import (
	"testing"

	webrtc "github.com/s4wave/spacewave/net/transport/webrtc"
)

func TestToWebRtcConfigurationICECandidatePoolSize(t *testing.T) {
	tests := []struct {
		name     string
		poolSize uint32
		want     uint8
	}{
		{name: "unset", want: 0},
		{name: "set", poolSize: 42, want: 42},
		{name: "out of range", poolSize: uint32(^uint8(0)) + 1, want: ^uint8(0)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := (&webrtc.WebRtcConfig{IceCandidatePoolSize: tt.poolSize}).ToWebRtcConfiguration()
			if config.ICECandidatePoolSize != tt.want {
				t.Errorf("ICECandidatePoolSize = %d, want %d", config.ICECandidatePoolSize, tt.want)
			}
		})
	}
}
