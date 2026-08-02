//go:build js

package opfs

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/coord/conformance"
	coord_inmem "github.com/s4wave/spacewave/db/coord/inmem"
	"github.com/s4wave/spacewave/db/volume/js/opfs/metashard"
)

func TestCoordinatorConformance(t *testing.T) {
	inner := coord_inmem.NewCoordinator()
	conformance.Check(t, func(testing.TB) (coord.Coordinator, coord.Coordinator) {
		return NewCoordinator(nil, "spacewave/test-volume", inner),
			NewCoordinator(nil, "spacewave/test-volume", inner)
	})
}

func TestKeyedCapabilityDoesNotReadGenerationStore(t *testing.T) {
	c := NewCoordinator(&metashard.MetaShard{}, "spacewave/test-volume", nil)
	capability, err := c.Capability(context.Background(), coord.Scope{
		VolumeID: "volume-a",
		Key:      "world-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !capability.Supported || capability.Generations {
		t.Fatalf("unexpected keyed capability: %+v", capability)
	}
}
