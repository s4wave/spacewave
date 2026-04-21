package forge_lib_git

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	git_clone "github.com/s4wave/spacewave/forge/lib/git/clone"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(git_clone.NewFactory(b))
}
