//go:build js

package core

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_indexeddb "github.com/s4wave/spacewave/db/volume/js/indexeddb"
)

// addNativeFactories adds factories specific to this platform.
func addNativeFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_indexeddb.NewFactory(b))
}
