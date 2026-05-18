package optypes

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// BuildSpaceLookupOp builds the Space world operation lookup chain.
func BuildSpaceLookupOp(b bus.Bus, le *logrus.Entry, engineID string) world.LookupOp {
	lookupOps := []world.LookupOp{LookupWorldOp}
	if b != nil {
		lookupOps = append(lookupOps, world.BuildLookupWorldOpFunc(b, le, engineID))
	}
	return world.NewLookupOpFromSlice(lookupOps)
}
