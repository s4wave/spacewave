//go:build !js

package bldr_core_devtool

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
)

// AddFactories adds the devtool factories.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	// volumes
	sr.AddFactory(volume_bolt.NewFactory(b))

	addCommonFactories(b, sr)
}
