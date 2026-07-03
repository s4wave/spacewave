package forge_lib_docker

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(NewFactory(b))
}
