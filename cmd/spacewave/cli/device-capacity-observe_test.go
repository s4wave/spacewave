package spacewave_cli

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"

	device_policy "github.com/s4wave/spacewave/core/device/policy"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

// newObserveTestbed builds a local world engine plus admission owner and
// returns a helper reading one Worker's capacity record (nil when absent).
func newObserveTestbed(t *testing.T) (context.Context, *forge_runtime.WorldRuntimeAdmission, func(key string) *forge_runtime.WorkerCapacity) {
	t.Helper()
	ctx := context.Background()
	btb, err := hydra_testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { btb.Release() })
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wtb.Release() })

	admission := forge_runtime.NewWorldRuntimeAdmission(wtb.Engine, nil, time.Minute, time.Minute)
	readCapacity := func(key string) *forge_runtime.WorkerCapacity {
		t.Helper()
		var out *forge_runtime.WorkerCapacity
		err := world.ExecTransaction(ctx, wtb.Engine, false, func(ctx context.Context, ws world.WorldState) error {
			capacity, err := forge_runtime.LookupWorkerCapacity(ctx, ws, key)
			if errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
				return nil
			}
			if err != nil {
				return err
			}
			out = capacity
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
	return ctx, admission, readCapacity
}

// declaredEnvelope builds the declared capacity values for one key.
func declaredEnvelope(key string) *device_policy.ForgeWorkerPolicy {
	return &device_policy.ForgeWorkerPolicy{
		WorkerObjectKey: key,
		MilliCpu:        1_000,
		MemoryBytes:     1 << 30,
		Backends:        []string{"docker"},
	}
}

// observeLogger returns a discarded logger for observer cycles.
func observeLogger() *logrus.Entry { return logrus.NewEntry(logrus.New()) }

// TestApplyDeclaredCapacityClaimObserveAndKeyChange pins the observer cycle:
// first declaration claims and observes at ACTIVE; a key change drains the
// stale record (holding its debit until release) and observes the new one.
func TestApplyDeclaredCapacityClaimObserveAndKeyChange(t *testing.T) {
	ctx, admission, readCapacity := newObserveTestbed(t)
	ref := forge_runtime.WorkerClaimRef{DeviceObjectKey: "devices/self", ClaimID: "claim-1"}
	le := observeLogger()

	mustDeclareForTest(t, ctx, admission, ref, le, "worker/a")
	capacity := readCapacity("worker/a")
	if capacity.OwnerState != forge_runtime.CapacityOwnerStateActive ||
		capacity.MilliCPUTotal != 1_000 || capacity.MilliCPUReserved != 0 {
		t.Fatalf("declared envelope not observed: %+v", capacity)
	}
	firstEpoch := capacity.OwnerEpoch

	res, err := admission.Reserve(ctx, "worker/a", "exec/key-change", testCapacityRequest())
	if err != nil {
		t.Fatal(err)
	}

	if err := applyDeclaredCapacity(ctx, le, admission, ref, declaredEnvelope("worker/b")); err != nil {
		t.Fatal(err)
	}
	oldRecord := readCapacity("worker/a")
	if oldRecord.OwnerState != forge_runtime.CapacityOwnerStateDraining || len(oldRecord.Backends) != 0 {
		t.Fatalf("key change must drain the stale record: %+v", oldRecord)
	}
	if oldRecord.MilliCPUReserved != testCapacityRequest().MilliCPU {
		t.Fatalf("stale record must hold its debit across key change: %+v", oldRecord)
	}
	newRecord := readCapacity("worker/b")
	if newRecord.OwnerState != forge_runtime.CapacityOwnerStateActive ||
		newRecord.MilliCPUTotal != 1_000 {
		t.Fatalf("new key not observed: %+v", newRecord)
	}

	// Releasing the old reservation lands the credit but never reactivates
	// the empty-backend drained record.
	if _, err := admission.StopAndRelease(ctx, ref, firstEpoch, res.ObjectKey(), res.Generation); err != nil {
		t.Fatal(err)
	}
	finalOld := readCapacity("worker/a")
	if finalOld.OwnerState != forge_runtime.CapacityOwnerStateDraining ||
		finalOld.MilliCPUReserved != 0 {
		t.Fatalf("empty-backend record must stay draining after credit: %+v", finalOld)
	}
}

// TestRemovalCycleDrainsThenCompletes pins removal: a nil declaration drains
// every owned record, holds through any live reservation, and deletes them
// once their reservations are terminal.
func TestRemovalCycleDrainsThenCompletes(t *testing.T) {
	ctx, admission, readCapacity := newObserveTestbed(t)
	ref := forge_runtime.WorkerClaimRef{DeviceObjectKey: "devices/self", ClaimID: "claim-1"}
	le := observeLogger()

	mustDeclareForTest(t, ctx, admission, ref, le, "worker/a")
	if _, err := admission.Reserve(ctx, "worker/a", "exec/removal", testCapacityRequest()); err != nil {
		t.Fatal(err)
	}
	epoch := readCapacity("worker/a").OwnerEpoch

	// Removal drains and blocks new work while the reservation stays live.
	if err := applyDeclaredCapacity(ctx, le, admission, ref, nil); err != nil {
		t.Fatal(err)
	}
	record := readCapacity("worker/a")
	if record == nil || record.OwnerState != forge_runtime.CapacityOwnerStateDraining {
		t.Fatalf("removal must drain the record: %+v", record)
	}
	if _, err := admission.Reserve(ctx, "worker/a", "exec/removal-2", testCapacityRequest()); !errors.Is(err, forge_runtime.ErrCapacityDraining) {
		t.Fatalf("drained record must refuse work, got %v", err)
	}

	// Terminal cleanup completes the drain and deletes the record once.
	if _, err := admission.StopAndRelease(ctx, ref, epoch, BuildReservationKeyForExec("exec/removal"), 1); err != nil {
		t.Fatal(err)
	}
	if err := applyDeclaredCapacity(ctx, le, admission, ref, nil); err != nil {
		t.Fatal(err)
	}
	if readCapacity("worker/a") != nil {
		t.Fatal("completed drain must delete the record")
	}
	if _, err := admission.Reserve(ctx, "worker/a", "exec/removal-3", testCapacityRequest()); !errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
		t.Fatalf("expected not-observed after deletion, got %v", err)
	}
}

// TestRenewOwnedCapacityOnceExtendsLiveClaim pins renewal between policy
// changes: the lease extends without an epoch bump while the claim is live.
func TestRenewOwnedCapacityOnceExtendsLiveClaim(t *testing.T) {
	ctx, admission, _ := newObserveTestbed(t)
	ref := forge_runtime.WorkerClaimRef{DeviceObjectKey: "devices/self", ClaimID: "claim-1"}
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", ref); err != nil {
		t.Fatal(err)
	}
	before, err := admission.ScanOwnedCapacity(ctx, ref.DeviceObjectKey)
	if err != nil || len(before) != 1 {
		t.Fatalf("expected one owned record: %+v err=%v", before, err)
	}
	if err := renewOwnedCapacityOnceWithAdmission(ctx, admission, ref.DeviceObjectKey, ref); err != nil {
		t.Fatal(err)
	}
	after, err := admission.ScanOwnedCapacity(ctx, ref.DeviceObjectKey)
	if err != nil || len(after) != 1 {
		t.Fatalf("expected one owned record: %+v err=%v", after, err)
	}
	if after[0].Capacity.OwnerEpoch != before[0].Capacity.OwnerEpoch {
		t.Fatalf("live renewal must not bump the epoch: %+v", after[0].Capacity)
	}
}

// mustDeclareForTest runs one declaration cycle and fails the test on error.
func mustDeclareForTest(
	t *testing.T,
	ctx context.Context,
	admission *forge_runtime.WorldRuntimeAdmission,
	ref forge_runtime.WorkerClaimRef,
	le *logrus.Entry,
	key string,
) {
	t.Helper()
	if err := applyDeclaredCapacity(ctx, le, admission, ref, declaredEnvelope(key)); err != nil {
		t.Fatal(err)
	}
}

// testCapacityRequest is a small request the declared envelope always fits.
func testCapacityRequest() forge_runtime.ResourceRequest {
	return forge_runtime.ResourceRequest{MilliCPU: 500, MemoryBytes: 1 << 29, Backend: "docker"}
}

// BuildReservationKeyForExec exposes reservation key derivation to tests.
func BuildReservationKeyForExec(executionObjectKey string) string {
	return forge_runtime.BuildReservationObjectKey(executionObjectKey)
}
