package forge_lib_v86

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	bun "github.com/s4wave/spacewave/forge/lib/v86/bun"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(bun.NewFactory(b))
}
