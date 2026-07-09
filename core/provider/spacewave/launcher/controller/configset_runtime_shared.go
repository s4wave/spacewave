package spacewave_launcher_controller

import (
	"strings"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
)

func filterBrowserLauncherConfigSet(c configset_proto.ConfigSetMap) configset_proto.ConfigSetMap {
	if len(c) == 0 {
		return c
	}
	out := make(configset_proto.ConfigSetMap, len(c))
	for key, conf := range c {
		if isBrowserReleaseWorldConfig(key, conf) {
			continue
		}
		out[key] = conf
	}
	return out
}

func isBrowserReleaseWorldConfig(key string, conf *configset_proto.ControllerConfig) bool {
	if strings.HasPrefix(key, "release-world") {
		return true
	}
	switch conf.GetId() {
	case "spacewave/cdn/world", "spacewave/cdn/bstore":
		return true
	default:
		return false
	}
}
