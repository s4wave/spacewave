package e2e_wasm_linksolicit

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/net/link/solicit/controller"
)

// NewFactory adapts the upstream no-arg factory to the bldr Go compiler's
// bus-parameter factory discovery path.
func NewFactory(b bus.Bus) controller.Factory {
	_ = b
	return link_solicit_controller.NewFactory()
}
