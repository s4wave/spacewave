package inmem

import (
	"testing"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
)

func TestCoordinatorConformance(t *testing.T) {
	conformance.Check(t, func(testing.TB) (coord.Coordinator, coord.Coordinator) {
		c := NewCoordinator()
		return c, c
	})
}
