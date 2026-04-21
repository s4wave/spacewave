//go:build !js

package plugin_entrypoint

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	plugin_host_storage "github.com/s4wave/spacewave/bldr/plugin/host/storage"
	"github.com/s4wave/spacewave/bldr/storage"
)

// buildPluginStorages builds the storage backends for the plugin.
// On native builds, uses the plugin host storage (RPC proxy) for cross-process access.
func buildPluginStorages(b bus.Bus, sr *static.Resolver) []storage.Storage {
	hostStorage := plugin_host_storage.NewPluginHostStorage()
	hostStorage.AddFactories(b, sr)
	return []storage.Storage{hostStorage}
}
