package space_world

import "github.com/s4wave/spacewave/db/world"

// WorldInternal marks Space settings internal: the Space keeps them, a person
// does not make them as an object.
func (*SpaceSettings) WorldInternal() {}

// _ is a type assertion
var _ world.InternalBlock = (*SpaceSettings)(nil)
