//go:build js && bldr_cloudflare

package plugin_entrypoint

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/bldr/storage"
)

// buildPluginStorages exposes no ambient browser or host storage in a facet.
func buildPluginStorages(bus.Bus, *static.Resolver) []storage.Storage { return nil }
