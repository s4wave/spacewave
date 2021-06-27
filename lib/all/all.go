package forge_lib_all

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	forge_kvtx "github.com/aperturerobotics/forge/lib/kvtx"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(forge_kvtx.NewFactory(b))
}
