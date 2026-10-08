package plugin_space

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

// TestReconcileProcessesOnlyOnChange checks that a World change outside the
// process bindings does not reconcile them again, while a notified binding
// change and the deletion of a bound object do.
func TestReconcileProcessesOnlyOnChange(t *testing.T) {
	// Bound the test and build the testbed.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Bind a World object to a process, outside the Space's running loop.
	const spaceID = "space-test"
	const objectKey = "test/bound"
	bound, err := tb.WorldState.CreateObject(ctx, objectKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(bound)
	store, releaseStore := buildBindingStore(ctx, t, tb)
	if err := process_binding.SetProcessBinding(ctx, store, spaceID, objectKey, &s4wave_process.ProcessBinding{
		State:     s4wave_process.ProcessBindingState_ProcessBindingState_UNAPPROVED,
		ObjectKey: objectKey,
		TypeId:    "test/process",
	}); err != nil {
		t.Fatal(err)
	}
	releaseStore()

	// Construct the Space controller without its watch loop, so the test calls
	// reconcileProcesses once per World change, and count its reconciles.
	logger, hook := logtest.NewNullLogger()
	logger.SetLevel(logrus.DebugLevel)
	reconciles := func() int {
		var n int
		for _, entry := range hook.AllEntries() {
			if entry.Message == "reconciling space processes" {
				n++
			}
		}
		return n
	}
	ctrl, err := NewFactory(tb.Bus).Construct(ctx, &Config{
		SpaceId:       spaceID,
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
	}, controller.ConstructOpts{Logger: logrus.NewEntry(logger)})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	space := ctrl.(*Controller)

	// The first reconcile reads the bindings.
	space.reconcileProcesses(ctx, tb.WorldState)
	if got := reconciles(); got != 1 {
		t.Fatalf("reconciles after the first reconcile = %d, want 1", got)
	}

	// A World change outside the bindings does not reread them.
	unrelated, err := tb.WorldState.CreateObject(ctx, "test/unrelated", nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(unrelated)
	space.reconcileProcesses(ctx, tb.WorldState)
	if got := reconciles(); got != 1 {
		t.Fatalf("reconciles after an unrelated change = %d, want 1", got)
	}

	// A notified binding change rereads them.
	space.NotifyChanged()
	space.reconcileProcesses(ctx, tb.WorldState)
	if got := reconciles(); got != 2 {
		t.Fatalf("reconciles after a binding change = %d, want 2", got)
	}

	// Deleting the bound object rereads them and deletes its binding.
	if _, err := tb.WorldState.DeleteObject(ctx, objectKey); err != nil {
		t.Fatal(err)
	}
	space.reconcileProcesses(ctx, tb.WorldState)
	if got := reconciles(); got != 3 {
		t.Fatalf("reconciles after deleting the bound object = %d, want 3", got)
	}
	store, releaseStore = buildBindingStore(ctx, t, tb)
	defer releaseStore()
	remaining, err := process_binding.ListProcessBindings(ctx, store, spaceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 0 {
		t.Fatalf("process bindings after deleting the bound object = %v, want none", remaining)
	}
}

// buildBindingStore opens the object store holding the process bindings.
func buildBindingStore(ctx context.Context, t *testing.T, tb *testbed.Testbed) (kvtx.Store, func()) {
	t.Helper()
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		tb.Bus,
		true,
		tb.EngineObjectStoreID,
		tb.EngineVolumeID,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	return handle.GetObjectStore(), ref.Release
}
