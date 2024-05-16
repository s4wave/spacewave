//go:build js

package hydra_cli_core

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	volume_indexeddb "github.com/aperturerobotics/hydra/volume/js/indexeddb"
	volume_kvtxinmem "github.com/aperturerobotics/hydra/volume/kvtxinmem"
)

// AddFactories adds the cli storage factories to the resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(volume_indexeddb.NewFactory(b))
	sr.AddFactory(volume_kvtxinmem.NewFactory(b))
}
