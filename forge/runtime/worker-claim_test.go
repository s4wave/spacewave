package forge_runtime

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// TestProductionCallerLifecycle proves the serving order a production caller
// runs: predeclare the execution key, claim the Worker, observe, reserve,
// activate, renew while active, then release on cancel.
func TestProductionCallerLifecycle(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute)
	const instance = "daemon-1"

	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", instance)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.ClaimInstance != instance || capacity.Draining {
		t.Fatalf("unexpected claim: %+v", capacity)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	executionObjectKey := "exec/1"
	res, err := admission.Reserve(ctx, "worker/a", executionObjectKey, testRequest)
	if err != nil {
		t.Fatal(err)
	}
	resKey := res.ObjectKey()
	rt := BackendRuntimeIdentity{Backend: "docker", ID: "container-1"}
	if _, err := admission.Activate(ctx, resKey, rt); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.RenewLease(ctx, resKey); err != nil {
		t.Fatal(err)
	}

	receipt, err := admission.StopAndRelease(ctx, resKey, res.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Complete() || receipt.Reason != "stopped" {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	finalCapacity := lookupCapacity(t, admission, "worker/a")
	if finalCapacity.MilliCPUReserved != 0 || finalCapacity.MemoryBytesReserved != 0 {
		t.Fatalf("debit leaked after release: %+v", finalCapacity)
	}
	if !finalCapacity.ClaimLive(instance, time.Now()) {
		t.Fatalf("claim lost after release: %+v", finalCapacity)
	}
}

// TestClaimFencesConcurrentInstances proves one live claim excludes a second
// owner and that an expired claim transfers with a clean drain state.
func TestClaimFencesConcurrentInstances(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute)
	now := time.Unix(1_700_000_000, 0)
	admission.SetTimeNow(func() time.Time { return now })

	const first, second = "daemon-1", "daemon-2"
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", first); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", second); !errors.Is(err, ErrWorkerClaimHeld) {
		t.Fatalf("expected ErrWorkerClaimHeld, got %v", err)
	}
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", first); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.RenewWorkerClaim(ctx, "worker/a", second); !errors.Is(err, ErrWorkerNotClaimed) {
		t.Fatalf("expected ErrWorkerNotClaimed, got %v", err)
	}

	now = now.Add(2 * time.Minute)
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", second)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.Draining || capacity.ClaimInstance != second {
		t.Fatalf("takeover did not reset drain state: %+v", capacity)
	}
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", second); !errors.Is(err, ErrDrainIncomplete) {
		t.Fatalf("complete without begin drain should fail, got %v", err)
	}
}

// TestCancelThenDrainRemovesRecord proves drain rejects new reserves, refuses
// to complete over live debits, and completes only after cancellation credits
// every reservation back.
func TestCancelThenDrainRemovesRecord(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute)
	const instance = "daemon-1"

	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", instance); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	resKey := res.ObjectKey()

	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", instance); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.RenewLease(ctx, resKey); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.Reserve(ctx, "worker/a", "exec/late", testRequest); !errors.Is(err, ErrWorkerDraining) {
		t.Fatalf("expected ErrWorkerDraining, got %v", err)
	}
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", instance); !errors.Is(err, ErrDrainIncomplete) {
		t.Fatalf("expected ErrDrainIncomplete over live debit, got %v", err)
	}

	receipt, err := admission.StopAndRelease(ctx, resKey, res.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Complete() {
		t.Fatalf("incomplete cancel receipt: %+v", receipt)
	}
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", instance); err != nil {
		t.Fatal(err)
	}
	err = admission.withTx(context.Background(), false, func(ctx context.Context, ws world.WorldState) error {
		_, err := LookupWorkerCapacity(ctx, ws, "worker/a")
		return err
	})
	if !errors.Is(err, ErrWorkerNotObserved) {
		t.Fatalf("expected drained record gone, got %v", err)
	}
}

// TestScanOwnedCapacity proves the scan returns only the calling instance's
// unexpired claims.
func TestScanOwnedCapacity(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute)
	now := time.Unix(1_700_000_000, 0)
	admission.SetTimeNow(func() time.Time { return now })

	for _, tc := range []struct{ worker, instance string }{
		{"worker/a", "daemon-1"},
		{"worker/b", "daemon-2"},
	} {
		if _, err := admission.ClaimWorkerCapacity(ctx, tc.worker, tc.instance); err != nil {
			t.Fatal(err)
		}
	}
	owned, err := admission.ScanOwnedCapacity(ctx, "daemon-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 1 || owned[0] != "worker/a" {
		t.Fatalf("unexpected owned set: %v", owned)
	}

	now = now.Add(2 * time.Minute)
	owned, err = admission.ScanOwnedCapacity(ctx, "daemon-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 0 {
		t.Fatalf("expired claims still scanned: %v", owned)
	}
}

// lookupCapacity reads one Worker capacity record through a read transaction.
func lookupCapacity(t *testing.T, admission *WorldRuntimeAdmission, workerObjectKey string) *WorkerCapacity {
	t.Helper()
	var capacity *WorkerCapacity
	err := admission.withTx(context.Background(), false, func(ctx context.Context, ws world.WorldState) error {
		var err error
		capacity, err = LookupWorkerCapacity(ctx, ws, workerObjectKey)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return capacity
}
