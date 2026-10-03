package forge_runtime

import (
	"testing"
	"time"

	"github.com/pkg/errors"
)

func TestReserveRejectsExpiredUnsweptIdempotentReturn(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Choose the time used to evaluate the Worker or reservation lease.
	now := time.Now().UTC()
	admission.SetTimeNow(func() time.Time { return now })

	// Reserve capacity for a new Execution attempt.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/late", testRequest); err != nil {
		t.Fatal(err)
	}

	// Advance past the lease without running the sweep: the idempotent return
	// must reject instead of re-arming a dead lease.
	admission.SetTimeNow(func() time.Time { return now.Add(2 * time.Minute) })

	// Reject reuse of the expired reservation before its sweep.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/late", testRequest); !errors.Is(err, ErrReservationExpired) {
		t.Fatalf("expected expired reservation error, got %v", err)
	}

	// The owner claim lapsed under the same shifted clock: renewal falls
	// back to reclaim with an epoch bump before the sweep can run.
	if _, err := admission.RenewWorkerClaim(ctx, "worker/a", selfRef); err != nil {
		t.Fatal(err)
	}

	// The sweep releases once; the attempt stays terminal for its key.
	receipts, err := admission.ExpireLeases(ctx, selfRef, now.Add(2*time.Minute))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("expected one expiry receipt: %+v err=%v", receipts, err)
	}

	// Reject reuse of the released Execution reservation.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/late", testRequest); !errors.Is(err, ErrReservationTerminal) {
		t.Fatalf("expected terminal error after sweep, got %v", err)
	}
}

// TestOldEpochMutationsFailAfterOwnerReclaim proves the owner-epoch fence:
// after reclaim following lease expiry, mutations carrying the old epoch fail
// deterministically and the reservation lease sweep belongs to the new epoch.
func TestOldEpochMutationsFailAfterOwnerReclaim(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	ref := WorkerClaimRef{DeviceObjectKey: "devices/self", ClaimID: "claim-1"}

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", ref)
	if err != nil {
		t.Fatal(err)
	}
	oldEpoch := capacity.OwnerEpoch

	// Choose the time used to evaluate the Worker or reservation lease.
	now := time.Now().UTC()
	admission.SetTimeNow(func() time.Time { return now.Add(2 * time.Minute) })

	// Reclaim the expired Worker capacity and check its new epoch.
	reclaimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", ref)
	if err != nil {
		t.Fatal(err)
	}
	if reclaimed.OwnerEpoch != oldEpoch+1 {
		t.Fatalf("reclaim must bump the epoch: %+v", reclaimed)
	}

	// Reject a Worker observation carrying the previous claim epoch.
	if _, err := admission.ObserveWorker(ctx, "worker/a", ref, oldEpoch, 2_000, 4<<30, []string{"docker"}); err == nil {
		t.Fatal("old-epoch observe must fail")
	}

	// Move the Worker capacity into draining.
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", ref, oldEpoch); err == nil {
		t.Fatal("old-epoch drain must fail")
	}
}
