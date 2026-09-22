//go:build !js

package dist_entrypoint

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
)

// addDesktopFactories registers controllers that must run in the installed
// desktop process rather than in one of its child plugins.
func addDesktopFactories(b bus.Bus, sr *static.Resolver) {
	// Installed hosts retain the application operation factory for local worlds.
	sr.AddFactory(space_world_optypes.NewFactory(b))

	// Replacement and relaunch target this process's executable and lifetime.
	sr.AddFactory(launcher.NewFactory(b))

	// The desktop host runs the status projector without the resource
	// listener controller; it projects a locally-owned status broker.
	sr.AddFactory(statusprojector.NewFactory(b))
}
