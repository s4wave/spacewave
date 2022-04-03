package forge_lib_all

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"

	forge_containers "github.com/aperturerobotics/forge/lib/containers"
	forge_git "github.com/aperturerobotics/forge/lib/git"
	forge_kvtx "github.com/aperturerobotics/forge/lib/kvtx"
)

// AddFactories adds factories to an existing static resolver.
func AddFactories(b bus.Bus, sr *static.Resolver) {
	forge_kvtx.AddFactories(b, sr)
	forge_git.AddFactories(b, sr)
	forge_containers.AddFactories(b, sr)
}
