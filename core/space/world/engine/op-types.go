// Package space_world_engine configures the World engine of a Space.
package space_world_engine

import (
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
)

// OpTypes resolves the operation types of a Space World: the built-in
// operations first, then the bus, and the operation types the Space's
// installed plugin manifests declare.
var OpTypes = space_world.OpTypes{
	Lookup:   space_world_optypes.LookupWorldOp,
	Declared: space_world.DeclaredOpPlugins,
}
