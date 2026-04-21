//go:build js

package bldr_core_devtool

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_indexeddb "github.com/s4wave/spacewave/db/volume/js/indexeddb"
)

// AddFactories adds the devtool factories.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	// volumes
	sr.AddFactory(volume_indexeddb.NewFactory(b))

	addCommonFactories(b, sr)
}
