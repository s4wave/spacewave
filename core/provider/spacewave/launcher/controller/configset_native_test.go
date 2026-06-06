//go:build !js && !goscript

package spacewave_launcher_controller

import (
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
)

func TestFilterRuntimeLauncherConfigSetKeepsSignedConfigSetOnNative(t *testing.T) {
	input := configset_proto.ConfigSetMap{
		"release-world": {
			Id:  "spacewave/cdn/world",
			Rev: 1,
		},
	}
	filtered := filterRuntimeLauncherConfigSet(input)
	if filtered["release-world"] == nil {
		t.Fatal("native launcher configset unexpectedly dropped release-world controller")
	}
}
