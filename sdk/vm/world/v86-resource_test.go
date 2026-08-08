package s4wave_vm_world

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	db_world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

func TestDefaultVmRuntimePluginID(t *testing.T) {
	if defaultVmPluginID != "spacewave-v86" {
		t.Fatalf("defaultVmPluginID = %q, want %q", defaultVmPluginID, "spacewave-v86")
	}
}

func TestV86RuntimeStatusGenerationFence(t *testing.T) {
	ctx := t.Context()
	wtb, err := db_world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
	const objectKey = "vm/v86/test"
	const generation = uint64(7)
	_, _, err = world.CreateWorldObject(ctx, ws, objectKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(&s4wave_vm.VmV86{
			State:         s4wave_vm.VmState_VmState_RUNNING,
			ObservedState: s4wave_vm.VmState_VmState_STARTING,
			RunGeneration: generation,
		}, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	resource := newV86Resource(nil, objectKey, ws, nil, nil)
	booting, err := resource.applyRuntimeStatus(ctx, generation, &s4wave_vm.ReportV86RuntimeStatusRequest{
		ObjectKey: objectKey,
		Status:    s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_BOOTING,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !booting.GetAccepted() || booting.GetRunGeneration() != generation {
		t.Fatalf("boot report = %#v, want accepted generation %d", booting, generation)
	}
	ready, err := resource.applyRuntimeStatus(ctx, generation, &s4wave_vm.ReportV86RuntimeStatusRequest{
		ObjectKey:     objectKey,
		RunGeneration: generation,
		Status:        s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_READY,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !ready.GetAccepted() {
		t.Fatalf("ready report rejected: %s", ready.GetRejection())
	}
	staleBootstrap, err := resource.applyRuntimeStatus(ctx, generation, &s4wave_vm.ReportV86RuntimeStatusRequest{
		ObjectKey: objectKey,
		Status:    s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_BOOTING,
	})
	if err != nil {
		t.Fatal(err)
	}
	if staleBootstrap.GetAccepted() {
		t.Fatal("bootstrap report regressed a ready runtime")
	}
	failed, err := resource.applyRuntimeStatus(ctx, generation, &s4wave_vm.ReportV86RuntimeStatusRequest{
		ObjectKey:     objectKey,
		RunGeneration: generation,
		Status:        s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_ERROR,
		ErrorMessage:  "forced runtime failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !failed.GetAccepted() {
		t.Fatalf("error report rejected: %s", failed.GetRejection())
	}

	stale, err := resource.applyRuntimeStatus(ctx, generation, &s4wave_vm.ReportV86RuntimeStatusRequest{
		ObjectKey:     objectKey,
		RunGeneration: generation - 1,
		Status:        s4wave_vm.V86RuntimeStatus_V86RuntimeStatus_ERROR,
		ErrorMessage:  "stale failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if stale.GetAccepted() {
		t.Fatal("stale runtime report was accepted")
	}
	changed, accepted, err := resource.updateObservedState(
		ctx,
		generation-1,
		s4wave_vm.VmState_VmState_ERROR,
		"stale failure",
	)
	if err != nil {
		t.Fatal(err)
	}
	if changed || accepted {
		t.Fatalf("stale state update = changed %t accepted %t", changed, accepted)
	}

	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("runtime object disappeared")
	}
	var vm *s4wave_vm.VmV86
	_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		blockValue, unmarshalErr := block.UnmarshalBlock[*s4wave_vm.VmV86](ctx, bcs, func() block.Block {
			return &s4wave_vm.VmV86{}
		})
		if unmarshalErr != nil {
			return unmarshalErr
		}
		vm = blockValue
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if vm == nil {
		t.Fatal("runtime object has no VmV86 block")
	}
	if vm.GetObservedState() != s4wave_vm.VmState_VmState_ERROR {
		t.Fatalf("observed state = %s, want ERROR", vm.GetObservedState())
	}
	if vm.GetErrorMessage() != "forced runtime failure" {
		t.Fatalf("error message = %q, want forced runtime failure", vm.GetErrorMessage())
	}
}
