//go:build !js && !wasip1

package bolt

import (
	"testing"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
)

func TestCoordinatorConformance(t *testing.T) {
	conformance.Check(t, func(tb testing.TB) (coord.Coordinator, coord.Coordinator) {
		inner := coord_inmem.NewCoordinator()
		return NewCoordinator(nil, inner), NewCoordinator(nil, inner)
	})
}
