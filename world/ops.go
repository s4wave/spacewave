package identity_world

import "github.com/aperturerobotics/hydra/world"

// LookupOp looks up any of the aperture-identity ops.
var LookupOp = world.NewLookupOpFromSlice([]world.LookupOp{
	LookupEntityOp,
	LookupKeypairOp,
	LookupDomainInfoOp,
})
