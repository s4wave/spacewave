package spacewave_launcher_controller

import configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"

func filterBrowserLauncherConfigSet(c configset_proto.ConfigSetMap) configset_proto.ConfigSetMap {
	if len(c) == 0 {
		return c
	}
	out := make(configset_proto.ConfigSetMap, len(c))
	for key, conf := range c {
		if isLegacyBrowserReleaseWorldConfig(conf) {
			continue
		}
		out[key] = conf
	}
	return out
}

func isLegacyBrowserReleaseWorldConfig(conf *configset_proto.ControllerConfig) bool {
	return conf.GetId() == "spacewave/cdn/world"
}
