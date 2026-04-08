//go:build js

package plugin_entrypoint

import (
	"github.com/aperturerobotics/bldr/storage"
	browser_storage "github.com/aperturerobotics/bldr/storage/browser"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
)

// buildPluginStorages builds the storage backends for the plugin.
// On js builds, uses direct OPFS access instead of proxying through the host.
func buildPluginStorages(b bus.Bus, sr *static.Resolver) []storage.Storage {
	storages := browser_storage.BuildStorage(b, "")
	for _, st := range storages {
		st.AddFactories(b, sr)
	}
	return storages
}
