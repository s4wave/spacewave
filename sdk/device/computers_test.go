package s4wave_device

import (
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/testbed"
)

func TestCreateComputersDashboardOpCreatesTypedDashboard(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	op := NewCreateComputersDashboardOp("computers", "Computers", time.Unix(100, 0))
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, op, ""); err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	typeID, err := world_types.GetObjectType(ctx, tb.WorldState, "computers")
	if err != nil {
		t.Fatalf("GetObjectType: %v", err)
	}
	if typeID != ComputersDashboardTypeID {
		t.Fatalf("type id = %q, want %q", typeID, ComputersDashboardTypeID)
	}

	obj, found, err := tb.WorldState.GetObject(ctx, "computers")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if !found {
		t.Fatal("computers object not found")
	}

	var dashboard *ComputersDashboard
	_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
		var uerr error
		dashboard, uerr = UnmarshalComputersDashboard(ctx, bcs)
		return uerr
	})
	if err != nil {
		t.Fatalf("AccessObjectState: %v", err)
	}
	if dashboard.GetName() != "Computers" {
		t.Fatalf("dashboard name = %q, want Computers", dashboard.GetName())
	}
}
