//go:build js

package dist_entrypoint

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
)

func addDesktopFactories(_ bus.Bus, _ *static.Resolver) {}
