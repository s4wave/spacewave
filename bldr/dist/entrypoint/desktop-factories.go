//go:build !js

package dist_entrypoint

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector"
	resource_listener "github.com/s4wave/spacewave/core/resource/listener"
)

func addDesktopFactories(b bus.Bus, sr *static.Resolver) {
	// The desktop host runs the status projector without the resource
	// listener controller; it projects a locally-owned status broker.
	sr.AddFactory(statusprojector.NewFactory(
		b,
		statusprojector.WithListenerStatusBroker(resource_listener.NewStatusBroker()),
	))
}
