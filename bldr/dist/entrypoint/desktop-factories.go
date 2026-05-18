//go:build !js

package dist_entrypoint

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/s4wave/spacewave/core/resource/desktop/statusprojector"
)

func addDesktopFactories(b bus.Bus, sr *static.Resolver) {
	sr.AddFactory(statusprojector.NewFactory(b))
}
