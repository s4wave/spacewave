//go:build !js

package webrtc

import "github.com/pion/webrtc/v4"

// applyICEOptions applies the pion-native ICE settings to the setting engine.
//
// The pion/ice network stack and consent timeouts only exist on the native
// build. The js build drives ICE through the browser RTCPeerConnection, where
// these setting-engine methods are absent, so applyICEOptions is a no-op there.
func applyICEOptions(se *webrtc.SettingEngine, o *options) {
	if o.iceNet != nil {
		se.SetNet(o.iceNet)
	}
	if o.iceDisconnectedTimeout != 0 || o.iceFailedTimeout != 0 || o.iceKeepaliveInterval != 0 {
		se.SetICETimeouts(o.iceDisconnectedTimeout, o.iceFailedTimeout, o.iceKeepaliveInterval)
	}
}
