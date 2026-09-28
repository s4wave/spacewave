package spacewave_launcher_controller

import (
	"strings"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
)

// filterRuntimeLauncherConfigSet preserves the producer-owned Release World on every
// runtime. Signed control pointers select releases, but cannot open a competing
// transport or replace the native reader of the host-owned block store.
func filterRuntimeLauncherConfigSet(c configset_proto.ConfigSetMap) configset_proto.ConfigSetMap {
	if len(c) == 0 {
		return c
	}
	out := make(configset_proto.ConfigSetMap, len(c))
	for key, conf := range c {
		if isReleaseWorldConfiguration(key, conf) {
			continue
		}
		out[key] = conf
	}
	return out
}

// isReleaseWorldConfiguration recognizes configs owned by the producer bootstrap.
func isReleaseWorldConfiguration(key string, conf *configset_proto.ControllerConfig) bool {
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
