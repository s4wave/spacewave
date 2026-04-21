package identity_world

import "github.com/s4wave/spacewave/db/world"

// LookupOp looks up any of the aperture-identity ops.
var LookupOp = world.NewLookupOpFromSlice([]world.LookupOp{
	LookupEntityOp,
	LookupKeypairOp,
	LookupDomainInfoOp,
})
