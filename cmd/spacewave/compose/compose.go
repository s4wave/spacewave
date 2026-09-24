// Package spacewave_compose composes the Spacewave host process for the
// generated CLI and distribution entrypoints.
package spacewave_compose

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/bldr/entrypoint/compose"
	cdn_bstore_controller "github.com/s4wave/spacewave/core/cdn/bstore/controller"
	cdn_world_controller "github.com/s4wave/spacewave/core/cdn/world/controller"
)

// Compose builds the Spacewave host composition. Call it once per process.
func Compose() *compose.Composition {
	// Every host reads the published Release World through the CDN.
	composition := &compose.Composition{
		Factories: []compose.AddFactoriesFunc{func(b bus.Bus) []controller.Factory {
			return []controller.Factory{
				cdn_world_controller.NewFactory(b),
				cdn_bstore_controller.NewFactory(b),
			}
		}},
	}

	// Native hosts add their process-owned controllers and commands.
	composeNative(composition)
	return composition
}
