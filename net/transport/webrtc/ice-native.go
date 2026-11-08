//go:build !js

package webrtc

import (
	"strings"

	"github.com/pion/webrtc/v4"
)

// skipInterfacePrefixes names the container and bridge interfaces native
// gathering skips by default.
var skipInterfacePrefixes = []string{"docker", "veth", "br-"}

// keepInterface reports whether native gathering uses the named interface by
// default.
func keepInterface(name string) bool {
	for _, prefix := range skipInterfacePrefixes {
		if strings.HasPrefix(name, prefix) {
			return false
		}
	}
	return true
}

// applyICEOptions applies the pion-native ICE settings to the setting engine.
//
// The pion/ice network stack and consent timeouts only exist on the native
// build. The js build drives ICE through the browser RTCPeerConnection, where
// these setting-engine methods are absent, so applyICEOptions is a no-op there.
func applyICEOptions(se *webrtc.SettingEngine, o *options, conf *Config) {
	// Skip interfaces that cannot reach another machine unless told otherwise.
	if !conf.GetGatherAllInterfaces() {
		se.SetInterfaceFilter(keepInterface)
	}

	// Apply the test seams.
	if o.iceNet != nil {
		se.SetNet(o.iceNet)
	}
	if o.iceDisconnectedTimeout != 0 || o.iceFailedTimeout != 0 || o.iceKeepaliveInterval != 0 {
		se.SetICETimeouts(o.iceDisconnectedTimeout, o.iceFailedTimeout, o.iceKeepaliveInterval)
	}
}
