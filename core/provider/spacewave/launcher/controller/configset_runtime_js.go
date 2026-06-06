//go:build js && !goscript

package spacewave_launcher_controller

import configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"

func filterRuntimeLauncherConfigSet(c configset_proto.ConfigSetMap) configset_proto.ConfigSetMap {
	return filterBrowserLauncherConfigSet(c)
}
