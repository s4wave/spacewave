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
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 1_000, 1<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/cancel-unreach", testRequest)
	if err != nil {
		t.Fatal(err)
	}
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
	if _, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1); err == nil {
		t.Fatal("expected stop failure to surface")
	}
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
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != testRequest.MilliCPU {
		t.Fatalf("cancel while unreachable must hold the debit: %+v", capacity)
	}

	// A repeated cancel for the same fenced generation stays inert while
	// the runtime remains unreachable.
	if _, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1); err == nil {
		t.Fatal("expected repeated unreachable stop failure to surface")
	}

	// The runtime becomes reachable; reconciliation confirms the stop once.
	stopper.failFor = ""
	done, err := admission.ReconcilePendingStops(ctx)
	if err != nil || len(done) != 1 || !done[0].Complete() {
		t.Fatalf("expected one completed receipt: %+v err=%v", done, err)
	}
	final, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if final.State != ReservationStateReleased || !final.Cleanup.Complete() {
		t.Fatalf("reservation not finalized after reconcile: %+v", final)
	}
	capacity, err = lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 0 {
		t.Fatalf("capacity must credit exactly once: %+v", capacity)
	}
	finalReceipt, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1)
	if err != nil || finalReceipt == nil || !finalReceipt.Complete() {
		t.Fatalf("expected terminal-idempotent receipt for the fencing generation: %+v err=%v", finalReceipt, err)
	}
	if _, err := admission.StopAndRelease(ctx, res.ObjectKey(), 2); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected post-release future-generation call to be stale, got %v", err)
	}
	if stopper.count(rt.ID) != 1 {
		t.Fatalf("stopper invoked more than once: %d", stopper.count(rt.ID))
	}
}
