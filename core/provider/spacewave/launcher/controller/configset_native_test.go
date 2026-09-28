//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
)

// TestFilterRuntimeLauncherConfigSetPreservesNativeAuthority prevents a signed
// config from mounting a second Release World over the build's native reader.
func TestFilterRuntimeLauncherConfigSetPreservesNativeAuthority(t *testing.T) {
	input := configset_proto.ConfigSetMap{
		"release-world": {
			Id:  "spacewave/cdn/world",
			Rev: 1,
		},
		"release-world-cdn-store": {Id: "spacewave/cdn/bstore", Rev: 1},
		"release-world-engine":    {Id: "hydra/world/block/engine", Rev: 1},
	}
	filtered := filterRuntimeLauncherConfigSet(input)
	if len(filtered) != 0 {
		t.Fatal("native signed config replaced the build's Release World authority")
	}
}
