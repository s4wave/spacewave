//go:build !js

package dist_entrypoint

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher/controller"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

// addDesktopFactories registers controllers that must run in the installed
// desktop process rather than in one of its child plugins.
func addDesktopFactories(b bus.Bus, sr *static.Resolver) {
	// Replacement and relaunch target this process's executable and lifetime.
	sr.AddFactory(launcher.NewFactory(b))

	// The desktop host runs the status projector without the resource
	// listener controller; it projects a locally-owned status broker.
	sr.AddFactory(statusprojector.NewFactory(
		b,
		statusprojector.WithListenerStatusBroker(resource_listener.NewStatusBroker()),
	))
}
