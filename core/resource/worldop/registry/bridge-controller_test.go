package resource_worldop_registry

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	cdc "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/s4wave/spacewave/db/world"
	s4wave_worldop_registry "github.com/s4wave/spacewave/sdk/worldop/registry"
	"github.com/sirupsen/logrus"
)

func TestWorldOpRegistryBridgeControllerPreservesEngineID(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(cdc.NewController(ctx, le))
	registry := NewWorldOpRegistryResource()
	registry.registrations[1] = &s4wave_worldop_registry.WorldOpRegistration{
		OperationTypeId: "spacewave-notes/notes/init-notebook",
		RegistrationId:  1,
		PluginId:        "spacewave-app",
	}

	ctrl := NewWorldOpRegistryBridgeController(le, b, registry)
	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatalf("AddController: %v", err)
	}
	defer rel()

	vs, _, ref, err := world.ExLookupWorldOp(
		ctx,
		b,
		le,
		"spacewave-notes/notes/init-notebook",
		"engine-123",
	)
	if err != nil {
		t.Fatalf("ExLookupWorldOp: %v", err)
	}
	defer ref.Release()
	if len(vs) != 1 {
		t.Fatalf("expected 1 lookup op, got %d", len(vs))
	}

	op, err := vs[0](ctx, "spacewave-notes/notes/init-notebook")
	if err != nil {
		t.Fatalf("lookup op: %v", err)
	}
	bridgeOp, ok := op.(*bridgeOperation)
	if !ok {
		t.Fatalf("expected *bridgeOperation, got %T", op)
	}
	if bridgeOp.engineID != "engine-123" {
		t.Fatalf("expected engineID engine-123, got %q", bridgeOp.engineID)
	}
}
