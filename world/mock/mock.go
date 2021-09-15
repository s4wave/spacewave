package world_mock

import (
	"github.com/aperturerobotics/hydra/world"
)

// LookupMockOp looks up an operation type for a op type id.
// returns nil, nil if not found.
var LookupMockOp = world.NewLookupOpFromSlice([]world.LookupOp{
	LookupMockObjectOp,
	LookupMockWorldOp,
})

// _ is a type assertion
var _ world.LookupOp = LookupMockOp
