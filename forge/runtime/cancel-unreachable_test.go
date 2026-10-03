package forge_runtime

import (
	"testing"
	"time"

	"github.com/pkg/errors"
)

// TestCancelWhileUncertainHoldsDebitUntilCleanupOrExpiry covers OQ-37
// cancel-while-unreachable: a caller cancels an Execution whose Device is
// disconnected. The cancellation stays pending, the debit stays held while
// the stop cannot confirm, and recovery releases capacity exactly once.
func TestCancelWhileUncertainHoldsDebitUntilCleanupOrExpiry(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 1_000, 1<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/cancel-unreach", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate the reservation with a backend runtime identity.
	rt := BackendRuntimeIdentity{Backend: "docker", ID: "container-unreach"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// The Device disconnects; custody goes uncertain with the debit held.
	if _, err := admission.MarkUncertain(ctx, res.ObjectKey()); err != nil {
		t.Fatal(err)
	}

	// Cancel arrives while the runtime is unreachable: the stop fails.
	stopper.failFor = rt.ID
	stopper.stopErr = errors.New("runtime unreachable")

	// Check the runtime stop against the reservation and Worker fences.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1); err == nil {
		t.Fatal("expected stop failure to surface")
	}

	// Read the durable reservation and check its custody state.
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationStatePendingStop {
		t.Fatalf("cancel while unreachable must leave a durable pending-stop: %+v", loaded)
	}
	if loaded.Cleanup != nil && (loaded.Cleanup.RuntimeStopped || loaded.Cleanup.CapacityReleased) {
		t.Fatalf("unconfirmed cancel must not record release facts: %+v", loaded.Cleanup)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != testRequest.MilliCPU {
		t.Fatalf("cancel while unreachable must hold the debit: %+v", capacity)
	}

	// A repeated cancel for the same fenced generation stays inert while
	// the runtime remains unreachable.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1); err == nil {
		t.Fatal("expected repeated unreachable stop failure to surface")
	}

	// The runtime becomes reachable; reconciliation confirms the stop once.
	stopper.failFor = ""
	done, err := admission.ReconcilePendingStops(ctx, selfRef)
	if err != nil || len(done) != 1 || !done[0].Complete() {
		t.Fatalf("expected one completed receipt: %+v err=%v", done, err)
	}

	// Read the durable reservation and check its custody state.
	final, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if final.State != ReservationStateReleased || !final.Cleanup.Complete() {
		t.Fatalf("reservation not finalized after reconcile: %+v", final)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err = lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 0 {
		t.Fatalf("capacity must credit exactly once: %+v", capacity)
	}
	finalReceipt, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1)
	if err != nil || finalReceipt == nil || !finalReceipt.Complete() {
		t.Fatalf("expected terminal-idempotent receipt for the fencing generation: %+v err=%v", finalReceipt, err)
	}

	// Check the runtime stop against the reservation and Worker fences.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 2); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected post-release future-generation call to be stale, got %v", err)
	}
	if stopper.count(rt.ID) != 1 {
		t.Fatalf("stopper invoked more than once: %d", stopper.count(rt.ID))
	}
}

// TestStaleRefStopAndReleaseRejectedWithoutStopper proves the pre-stop fence:
// a deposed instance holding the old owner epoch cannot stop runtimes or
// credit capacity; the stopper must not fire.
func TestStaleRefStopAndReleaseRejectedWithoutStopper(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute, time.Minute)
	ref := WorkerClaimRef{DeviceObjectKey: "devices/self", ClaimID: "claim-1"}

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", ref)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", ref, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/cancel-1", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate the reservation with a backend runtime identity.
	rt := BackendRuntimeIdentity{Backend: "docker", ID: "container-cancel"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// Depose the instance: same Device, new claim id bumps the epoch.
	newRef := WorkerClaimRef{DeviceObjectKey: ref.DeviceObjectKey, ClaimID: "claim-2"}
	fresh, err := admission.ClaimWorkerCapacity(ctx, "worker/a", newRef)
	if err != nil {
		t.Fatal(err)
	}

	// The deposed epoch cannot stop the runtime it no longer owns.
	if _, err := admission.StopAndRelease(ctx, ref, capacity.OwnerEpoch, res.ObjectKey(), res.Generation); err == nil {
		t.Fatal("deposed instance stop must fail")
	}
	if stopper.count("container-cancel") != 0 {
		t.Fatal("rejected stop still invoked the stopper")
	}

	// The new owner stops the runtime with its own epoch.
	if _, err := admission.StopAndRelease(ctx, newRef, fresh.OwnerEpoch, res.ObjectKey(), res.Generation); err != nil {
		t.Fatal(err)
	}
	if stopper.count("container-cancel") != 1 {
		t.Fatalf("expected exactly one stop, got %d", stopper.count("container-cancel"))
	}
}
