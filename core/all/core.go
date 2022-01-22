package core_all

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/hydra/core"
	api_controller "github.com/aperturerobotics/hydra/daemon/api/controller"
)

// AddFactories adds all factories (including World Graph) to the static resolver.
// This is intended to keep the default Core as minimal as possible.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	core.AddFactories(b, sr)
	sr.AddFactory(api_controller.NewFactory(b))
}
