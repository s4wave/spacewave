package optypes

import (
	"context"
	"testing"

	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

func TestBuildSpaceLookupOpResolvesBuiltInWithoutBus(t *testing.T) {
	lookupOp := BuildSpaceLookupOp(nil, nil, "space/local/test")

	op, err := lookupOp(context.Background(), space_world_ops.InitUnixFSOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*space_world_ops.InitUnixFSOp); !ok {
		t.Fatalf("expected InitUnixFSOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), s4wave_wizard.CreateWizardObjectOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_wizard.CreateWizardObjectOp); !ok {
		t.Fatalf("expected CreateWizardObjectOp, got %T", op)
	}

	op, err = lookupOp(context.Background(), s4wave_device.CreateComputersDashboardOpId)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := op.(*s4wave_device.CreateComputersDashboardOp); !ok {
		t.Fatalf("expected CreateComputersDashboardOp, got %T", op)
	}
}

func TestBuildSpaceLookupOpReturnsNilForUnknownWithoutBus(t *testing.T) {
	lookupOp := BuildSpaceLookupOp(nil, nil, "space/local/test")

	op, err := lookupOp(context.Background(), "space/world/custom-op")
	if err != nil {
		t.Fatal(err)
	}
	if op != nil {
		t.Fatalf("expected nil op, got %T", op)
	}
}
