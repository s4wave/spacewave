// Package space_world_engine configures the World engine of a Space.
package space_world_engine

import (
	"github.com/aperturerobotics/controllerbus/bus"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
)

// OpTypes resolves the operation types of a Space World: the built-in
// operations first, then the handlers registered on the bus.
var OpTypes = space_world.OpTypes{
	Lookup: space_world_optypes.LookupWorldOp,
}

// NewFactory returns the Space World engine factory, which resolves operation
// types with OpTypes.
func NewFactory(b bus.Bus) *space_world.Factory {
	return space_world.NewEngineFactory(b, OpTypes)
}
