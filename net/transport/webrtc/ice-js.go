//go:build js

package webrtc

import "github.com/pion/webrtc/v4"

// applyICEOptions is a no-op on the js build, where the browser
// RTCPeerConnection owns ICE and the pion-native network and consent-timeout
// setting-engine methods do not exist.
func applyICEOptions(_ *webrtc.SettingEngine, _ *options) {}
