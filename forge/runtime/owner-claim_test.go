package forge_runtime

import (
	"testing"
	"time"

	"github.com/pkg/errors"
)

// selfRef is the owning instance claim used across the owner-state tests.
var selfRef = WorkerClaimRef{DeviceObjectKey: "devices/self", ClaimID: "claim-1"}

// otherRef is a foreign Device claim used to prove cross-instance rejection.
var otherRef = WorkerClaimRef{DeviceObjectKey: "devices/other", ClaimID: "claim-x"}

func TestClaimCreateRenewAndReclaimPreserveDraining(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Create-on-claim: absent record starts owned at epoch 1, state ACTIVE.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.OwnerEpoch != 1 || capacity.OwnerState != CapacityOwnerStateActive {
		t.Fatalf("unexpected created claim: %+v", capacity)
	}
	if capacity.WorkerObjectKey != "worker/a" || capacity.OwnerDeviceObjectKey != selfRef.DeviceObjectKey {
		t.Fatalf("claim did not persist key fields: %+v", capacity)
	}

	// Same ref renews idempotently without bumping the epoch.
	capacity, err = admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil || capacity.OwnerEpoch != 1 {
		t.Fatalf("same-ref claim must renew in place: %+v err=%v", capacity, err)
	}

	// Drain, then let the lease lapse; reclaim must keep DRAINING so a
	// drained worker never silently re-enters scheduling.
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}

	// Choose the time used to evaluate the Worker or reservation lease.
	now := time.Now().UTC()
	admission.SetTimeNow(func() time.Time { return now.Add(2 * time.Minute) })
	capacity, err = admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.OwnerEpoch != 2 || capacity.OwnerState != CapacityOwnerStateDraining {
		t.Fatalf("reclaim must bump epoch and preserve DRAINING: %+v", capacity)
	}
}

// TestClaimFencesReserveAndActivate proves a replaced Worker cannot debit or
// activate capacity after a new owner epoch takes custody.
func TestClaimFencesReserveAndActivate(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.ReserveForClaim(ctx, "worker/a", "exec/one", testRequest, selfRef, capacity.OwnerEpoch)
	if err != nil {
		t.Fatal(err)
	}

	// Replace the Worker claim with a new instance of the same Device.
	newRef := WorkerClaimRef{DeviceObjectKey: selfRef.DeviceObjectKey, ClaimID: "replacement"}
	newCapacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", newRef)
	if err != nil {
		t.Fatal(err)
	}
	if newCapacity.OwnerEpoch == capacity.OwnerEpoch {
		t.Fatal("replacement kept the old owner epoch")
	}

	// Reject a reservation from the replaced Worker claim.
	if _, err := admission.ReserveForClaim(ctx, "worker/a", "exec/two", testRequest, selfRef, capacity.OwnerEpoch); !errors.Is(err, ErrCapacityOwned) {
		t.Fatalf("old owner reserved capacity: %v", err)
	}

	// Activate runtime custody for the reserved Execution.
	if _, err := admission.ActivateForClaim(ctx, res.ObjectKey(), BackendRuntimeIdentity{Backend: "docker", ID: "stale"}, selfRef, capacity.OwnerEpoch); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("old owner activated runtime: %v", err)
	}
}

func TestForeignLiveClaimRejected(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for its test Device.
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef); err != nil {
		t.Fatal(err)
	}

	// Reject a foreign Device claim while the Worker lease is live.
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", otherRef); !errors.Is(err, ErrCapacityOwned) {
		t.Fatalf("expected foreign Device conflict, got %v", err)
	}
}

// TestExpiredOwnerClaimRejectsNewReservation checks that an unswept capacity
// record stops admitting work at its persisted owner lease deadline.
func TestExpiredOwnerClaimRejectsNewReservation(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, DefaultOwnerLeaseDuration)

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	admission.SetTimeNow(func() time.Time { return capacity.OwnerLeaseExpiresAt.AsTime() })

	// Reject reservations after the Worker claim lease expires.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/after-expiry", testRequest); !errors.Is(err, ErrCapacityOwnerExpired) {
		t.Fatalf("expired owner admitted work: %v", err)
	}
}

func TestEpochFencesObserveDrainAndComplete(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	stale := capacity.OwnerEpoch

	// A new claim id on the same Device reclaims and bumps the epoch; every
	// stale-epoch mutation from the replaced instance rejects without
	// touching the record.
	newRef := WorkerClaimRef{DeviceObjectKey: selfRef.DeviceObjectKey, ClaimID: "claim-2"}
	capacity, err = admission.ClaimWorkerCapacity(ctx, "worker/a", newRef)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.OwnerEpoch != stale+1 {
		t.Fatalf("reclaim must bump the epoch: %+v", capacity)
	}

	// Reject a Worker observation carrying the previous claim epoch.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, stale, 2_000, 4<<30, []string{"docker"}); err == nil {
		t.Fatal("stale-epoch observe must fail")
	}

	// Move the Worker capacity into draining.
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, stale); err == nil {
		t.Fatal("stale-epoch drain must fail")
	}

	// Check whether the draining Worker can remove its capacity record.
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", selfRef, stale); err == nil {
		t.Fatal("stale-epoch complete-drain must fail")
	}

	// Check the runtime stop against the reservation and Worker fences.
	if _, err := admission.StopAndRelease(ctx, selfRef, stale, "forge/runtime/reservation/x", 1); err == nil {
		t.Fatal("stale-epoch stop must fail")
	}
}

func TestRenewAfterExpiryFallsBackToReclaimWithBump(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Choose the time used to evaluate the Worker or reservation lease.
	now := time.Now().UTC()
	admission.SetTimeNow(func() time.Time { return now.Add(2 * time.Minute) })
	renewed, err := admission.RenewWorkerClaim(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if renewed.OwnerEpoch != capacity.OwnerEpoch+1 {
		t.Fatalf("expired renewal must fall back to reclaim with an epoch bump: %+v", renewed)
	}
}

func TestReserveRejectionPrecedenceUnderClaims(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Draining record with fitting totals still rejects new work.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Move the Worker capacity into draining.
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}

	// Reject reservations while the Worker capacity is draining.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/2", testRequest); !errors.Is(err, ErrCapacityDraining) {
		t.Fatalf("expected draining rejection, got %v", err)
	}
}

func TestCompleteDrainWaitsForTerminalThenDeletesOnce(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Move the Worker capacity into draining.
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}

	// A live reservation blocks completion.
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err == nil {
		t.Fatal("complete-drain must wait for terminal reservations")
	}

	// Check the runtime stop against the reservation and Worker fences.
	if _, err := admission.StopAndRelease(ctx, selfRef, capacity.OwnerEpoch, res.ObjectKey(), res.Generation); err != nil {
		t.Fatal(err)
	}

	// Check whether the draining Worker can remove its capacity record.
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}

	// Post-delete stragglers are impossible: the record is gone.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/3", testRequest); !errors.Is(err, ErrWorkerNotObserved) {
		t.Fatalf("expected not-observed after deletion, got %v", err)
	}
}

func TestStaleRefSweepsSkipWithoutInvokingStopper(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate runtime custody for the reserved Execution.
	if _, err := admission.Activate(ctx, res.ObjectKey(), BackendRuntimeIdentity{Backend: "docker", ID: "container-1"}); err != nil {
		t.Fatal(err)
	}

	// Choose the time used to evaluate the Worker or reservation lease.
	now := time.Now().UTC()

	// A foreign ref must not expire or stop work it does not own; the
	// stopper must never fire for it.
	if _, err := admission.ExpireLeases(ctx, otherRef, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if stopper.count("container-1") != 0 {
		t.Fatal("foreign sweep invoked the stopper")
	}

	// The matching owner sweeps normally and fences the generation.
	receipts, err := admission.ExpireLeases(ctx, selfRef, now.Add(2*time.Minute))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("expected one expiry receipt: %+v err=%v", receipts, err)
	}
	if receipts[0].Generation != res.Generation+1 {
		t.Fatalf("expiry must fence the generation: %+v", receipts[0])
	}
}

func TestScanOwnedCapacityReturnsKeysAndRecords(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim two Worker capacity records for the test Device.
	for _, key := range []string{"worker/a", "worker/b"} {
		if _, err := admission.ClaimWorkerCapacity(ctx, key, selfRef); err != nil {
			t.Fatal(err)
		}
	}

	// Claim the Worker capacity for its test Device.
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/other", otherRef); err != nil {
		t.Fatal(err)
	}

	// Scan Worker capacities belonging to the test Device.
	owned, err := admission.ScanOwnedCapacity(ctx, selfRef.DeviceObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 2 {
		t.Fatalf("expected two owned records, got %+v", owned)
	}

	// Check that the capacity scan pairs both Worker keys with their records.
	keys := map[string]bool{}
	for _, oc := range owned {
		if oc.WorkerObjectKey == "" || oc.Capacity == nil {
			t.Fatalf("scan must pair key with record: %+v", oc)
		}
		keys[oc.WorkerObjectKey] = true
	}
	if !keys["worker/a"] || !keys["worker/b"] {
		t.Fatalf("scan missed owned workers: %+v", keys)
	}
}

func TestGatedRenewLeaseRejectsStaleRef(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Reject reservation renewal from a foreign Worker claim.
	if _, err := admission.RenewLease(ctx, otherRef, res.ObjectKey()); err == nil {
		t.Fatal("deposed-instance lease renewal must be rejected")
	}

	// Renew the reservation under the current Worker claim.
	if _, err := admission.RenewLease(ctx, selfRef, res.ObjectKey()); err != nil {
		t.Fatal(err)
	}
}

// TestObserveFitDrainsAndCreditReactivates pins the desired-state write and
// the credit-only reactivation rule end to end.
func TestObserveFitDrainsAndCreditReactivates(t *testing.T) {
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

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/fit", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Shrink below the debit: observe drains and blocks new work.
	shrunk, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 500, 1<<29, []string{"docker"})
	if err != nil {
		t.Fatal(err)
	}
	if shrunk.OwnerState != CapacityOwnerStateDraining {
		t.Fatalf("shrink below debits must drain: %+v", shrunk)
	}

	// Reject reservations while the Worker capacity is draining.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/fit-2", testRequest); !errors.Is(err, ErrCapacityDraining) {
		t.Fatalf("drained record must refuse work: %v", err)
	}

	// A fitting observation reactivates directly.
	fit, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"})
	if err != nil {
		t.Fatal(err)
	}
	if fit.OwnerState != CapacityOwnerStateActive {
		t.Fatalf("fitting totals must reactivate on observe: %+v", fit)
	}

	// Shrink again, then credit via terminal release: reactivation happens in
	// creditCapacity because the declared backends remain non-empty.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 500, 1<<29, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Stop the backend runtime and check the cleanup receipt.
	receipt, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), res.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Complete() {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	final, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if final.OwnerState != CapacityOwnerStateActive {
		t.Fatalf("credit must reactivate a drained-with-backends record: %+v", final)
	}

	// Empty backends never self-activate: drain empties them, a later credit
	// keeps the record draining until CompleteDrain deletes it. Restore
	// fitting totals first so the reservation succeeds.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res2, err := admission.Reserve(ctx, "worker/a", "exec/fit-3", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Move the Worker capacity into draining.
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, claimed.OwnerEpoch); err != nil {
		t.Fatal(err)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	drained, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil || len(drained.Backends) != 0 {
		t.Fatalf("begin-drain must empty backends: %+v err=%v", drained, err)
	}

	// Check the runtime stop against the reservation and Worker fences.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res2.ObjectKey(), res2.Generation); err != nil {
		t.Fatal(err)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	final, err = lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if final.OwnerState != CapacityOwnerStateDraining || len(final.Backends) != 0 {
		t.Fatalf("empty-backend record must never self-activate: %+v", final)
	}
}

// TestEmptyBackendsObserveRejected pins that observations cannot clear the
// backend list; only BeginDrainCapacity drains with empty backends.
func TestEmptyBackendsObserveRejected(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Reject a Worker observation without supported runtime backends.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, nil); err == nil {
		t.Fatal("nil backends observation must be rejected")
	}

	// Reject a Worker observation without supported runtime backends.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{}); err == nil {
		t.Fatal("empty backends observation must be rejected")
	}
}

// TestStaleClaimIdentitiesAreStaleGeneration pins that both a foreign claim id
// on the same Device and an old epoch surface as ErrStaleGeneration for gated
// mutations, and that stale-ref reconcile never invokes the stopper.
func TestStaleClaimIdentitiesAreStaleGeneration(t *testing.T) {
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
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/stale", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate the reservation with a backend runtime identity.
	rt := BackendRuntimeIdentity{Backend: "docker", ID: "container-stale"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// Turnover: same Device, new claim id bumps the epoch.
	newRef := WorkerClaimRef{DeviceObjectKey: selfRef.DeviceObjectKey, ClaimID: "claim-new"}
	fresh, err := admission.ClaimWorkerCapacity(ctx, "worker/a", newRef)
	if err != nil {
		t.Fatal(err)
	}
	staleRef := WorkerClaimRef{DeviceObjectKey: selfRef.DeviceObjectKey, ClaimID: selfRef.ClaimID}

	// Old claim id and old epoch both fence to ErrStaleGeneration.
	if _, err := admission.ObserveWorker(ctx, "worker/a", staleRef, fresh.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("old claim id must be stale generation, got %v", err)
	}

	// Reject a Worker observation carrying the previous claim epoch.
	if _, err := admission.ObserveWorker(ctx, "worker/a", newRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("old epoch must be stale generation, got %v", err)
	}

	// Expire past the reservation lease with the stopper failing so the
	// pending-stop stays durable across reconciliation attempts. Reconcile
	// belongs to the new owner.
	now := time.Now().UTC()
	later := now.Add(2 * time.Minute)
	admission.SetTimeNow(func() time.Time { return later })
	stopper.failFor = "container-stale"
	stopper.stopErr = errors.New("runtime unreachable")
	if _, err := admission.RenewWorkerClaim(ctx, "worker/a", newRef); err != nil {
		t.Fatal(err)
	}

	// Sweep expired reservation leases and check the cleanup receipts.
	receipts, err := admission.ExpireLeases(ctx, newRef, later)
	if err != nil || len(receipts) != 1 || receipts[0].Complete() {
		t.Fatalf("expected partial expiry receipt: %+v err=%v", receipts, err)
	}
	stoppedBefore := stopper.count("container-stale")

	// The deposed instance's reconcile must neither stop the runtime nor
	// touch state; the stopper count proves the pre-stop fence.
	done, err := admission.ReconcilePendingStops(ctx, staleRef)
	if err != nil {
		t.Fatal(err)
	}
	if len(done) != 0 {
		t.Fatalf("stale ref reconcile must be skipped: %+v", done)
	}
	if stopper.count("container-stale") != stoppedBefore {
		t.Fatal("stale-ref reconcile invoked the stopper")
	}

	// The runtime recovers; the current owner reconciles and stops once.
	stopper.failFor = ""
	done, err = admission.ReconcilePendingStops(ctx, newRef)
	if err != nil || len(done) != 1 || !done[0].Complete() {
		t.Fatalf("expected completed receipt from live owner: %+v err=%v", done, err)
	}
	if stopper.count("container-stale") != stoppedBefore+1 {
		t.Fatalf("stopper invoked %d times", stopper.count("container-stale"))
	}
}
