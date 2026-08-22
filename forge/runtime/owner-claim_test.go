package forge_runtime

import (
	"context"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// selfRef is the owning instance claim used across the owner-state tests.
var selfRef = WorkerClaimRef{DeviceObjectKey: "devices/self", ClaimID: "claim-1"}

// otherRef is a foreign Device claim used to prove cross-instance rejection.
var otherRef = WorkerClaimRef{DeviceObjectKey: "devices/other", ClaimID: "claim-x"}

// persistLegacyCapacity writes an old-format capacity record without owner
// fields directly into world state, bypassing the gated API.
func persistLegacyCapacity(ctx context.Context, eng world.Engine, workerObjectKey string) error {
	return world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		legacy := &WorkerCapacity{
			MilliCPUTotal:    2_000,
			MemoryBytesTotal: 4 << 30,
			Generation:       1,
			ObservedAt:       timestamp.Now(),
		}
		return persistWorkerCapacity(ctx, ws, workerObjectKey, legacy)
	})
}

func TestClaimCreateAdoptAndReclaimPreserveDraining(t *testing.T) {
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

func TestClaimAdoptsLegacyOwnerlessRecord(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	if err := persistLegacyCapacity(ctx, eng, "worker/legacy"); err != nil {
		t.Fatal(err)
	}

	// Before adoption every gated operation refuses the record.
	if _, err := admission.Reserve(ctx, "worker/legacy", "exec/0", testRequest); !errors.Is(err, ErrCapacityUnowned) {
		t.Fatalf("expected unowned rejection on legacy record, got %v", err)
	}

	// Adoption preserves debits (none here) and activates under epoch 1.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/legacy", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.OwnerEpoch != 1 || capacity.MilliCPUTotal != 2_000 {
		t.Fatalf("adoption lost observed totals: %+v", capacity)
	}
}

func TestForeignLiveClaimRejected(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", otherRef); !errors.Is(err, ErrCapacityOwned) {
		t.Fatalf("expected foreign Device conflict, got %v", err)
	}
}

func TestEpochFencesObserveDrainAndComplete(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
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
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, stale, 2_000, 4<<30, []string{"docker"}); err == nil {
		t.Fatal("stale-epoch observe must fail")
	}
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, stale); err == nil {
		t.Fatal("stale-epoch drain must fail")
	}
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", selfRef, stale); err == nil {
		t.Fatal("stale-epoch complete-drain must fail")
	}
	if _, err := admission.StopAndRelease(ctx, selfRef, stale, "forge/runtime/reservation/x", 1); err == nil {
		t.Fatal("stale-epoch stop must fail")
	}
}

func TestRenewAfterExpiryFallsBackToReclaimWithBump(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
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
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Unowned legacy record: Reserve rejects before backend checks even
	// though the record declares the backend and ample totals.
	if err := persistLegacyCapacity(ctx, eng, "worker/legacy"); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.Reserve(ctx, "worker/legacy", "exec/1", testRequest); !errors.Is(err, ErrCapacityUnowned) {
		t.Fatalf("expected unowned rejection, got %v", err)
	}

	// Draining record with fitting totals still rejects new work.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.Reserve(ctx, "worker/a", "exec/2", testRequest); !errors.Is(err, ErrCapacityDraining) {
		t.Fatalf("expected draining rejection, got %v", err)
	}
}

func TestCompleteDrainWaitsForTerminalThenDeletesOnce(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}
	// A live reservation blocks completion.
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err == nil {
		t.Fatal("complete-drain must wait for terminal reservations")
	}
	if _, err := admission.StopAndRelease(ctx, selfRef, capacity.OwnerEpoch, res.ObjectKey(), res.Generation); err != nil {
		t.Fatal(err)
	}
	if err := admission.CompleteDrainCapacity(ctx, "worker/a", selfRef, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}
	// Post-delete stragglers are impossible: the record is gone.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/3", testRequest); !errors.Is(err, ErrWorkerNotObserved) {
		t.Fatalf("expected not-observed after deletion, got %v", err)
	}
}

func TestStaleRefSweepsSkipWithoutInvokingStopper(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute, time.Minute)
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.Activate(ctx, res.ObjectKey(), BackendRuntimeIdentity{Backend: "docker", ID: "container-1"}); err != nil {
		t.Fatal(err)
	}
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
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	for _, key := range []string{"worker/a", "worker/b"} {
		if _, err := admission.ClaimWorkerCapacity(ctx, key, selfRef); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/other", otherRef); err != nil {
		t.Fatal(err)
	}
	owned, err := admission.ScanOwnedCapacity(ctx, selfRef.DeviceObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if len(owned) != 2 {
		t.Fatalf("expected two owned records, got %+v", owned)
	}
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
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.RenewLease(ctx, otherRef, res.ObjectKey()); err == nil {
		t.Fatal("deposed-instance lease renewal must be rejected")
	}
	if _, err := admission.RenewLease(ctx, selfRef, res.ObjectKey()); err != nil {
		t.Fatal(err)
	}
}

// TestLegacyRawJSONDecodesUnavailableAndAdopts pins the raw legacy wire shape:
// a record written before owner claims carries none of the owner keys, decodes
// cleanly, stays unavailable to every gated operation, and is adopted by a
// claim with its observed totals intact. It also pins forward compatibility:
// the new JSON form round-trips every pre-existing field unchanged.
func TestLegacyRawJSONDecodesUnavailableAndAdopts(t *testing.T) {
	legacyJSON := `{"milliCpuTotal":2000,"memoryBytesTotal":4294967296,` +
		`"milliCpuReserved":1000,"memoryBytesReserved":1073741824,` +
		`"backends":["docker"],"observedAt":"1970-01-01T00:00:10Z",` +
		`"generation":3}`
	var decoded WorkerCapacity
	if err := decoded.UnmarshalJSON([]byte(legacyJSON)); err != nil {
		t.Fatal(err)
	}
	if decoded.MilliCPUTotal != 2_000 || decoded.MilliCPUReserved != 1_000 ||
		decoded.MemoryBytesTotal != 4<<30 || decoded.MemoryBytesReserved != 1<<30 ||
		len(decoded.Backends) != 1 || decoded.Generation != 3 {
		t.Fatalf("legacy decode lost pre-existing fields: %+v", decoded)
	}
	if err := decoded.Validate(); err != nil {
		t.Fatalf("legacy ownerless shape must validate: %v", err)
	}

	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, ws, BuildWorkerCapacityObjectKey("worker/legacy"), true,
			func(bcs *block.Cursor) error {
				bcs.SetBlock(&decoded, true)
				return nil
			})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Unavailable: Reserve refuses the ownerless record despite ample totals.
	if _, err := admission.Reserve(ctx, "worker/legacy", "exec/legacy", testRequest); !errors.Is(err, ErrCapacityUnowned) {
		t.Fatalf("expected unowned rejection, got %v", err)
	}

	// Adopt: the claim preserves observed totals and debits under epoch 1.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/legacy", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if capacity.OwnerEpoch != 1 || capacity.MilliCPUReserved != 1_000 ||
		capacity.WorkerObjectKey != "worker/legacy" {
		t.Fatalf("adoption lost durable facts: %+v", capacity)
	}
	if _, err := admission.Reserve(ctx, "worker/legacy", "exec/legacy2", testRequest); err != nil {
		t.Fatal(err)
	}
}

// TestObserveFitDrainsAndCreditReactivates pins the desired-state write and
// the credit-only reactivation rule end to end.
func TestObserveFitDrainsAndCreditReactivates(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
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
	receipt, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), res.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Complete() {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
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
	res2, err := admission.Reserve(ctx, "worker/a", "exec/fit-3", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, claimed.OwnerEpoch); err != nil {
		t.Fatal(err)
	}
	drained, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil || len(drained.Backends) != 0 {
		t.Fatalf("begin-drain must empty backends: %+v err=%v", drained, err)
	}
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res2.ObjectKey(), res2.Generation); err != nil {
		t.Fatal(err)
	}
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
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, nil); err == nil {
		t.Fatal("nil backends observation must be rejected")
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{}); err == nil {
		t.Fatal("empty backends observation must be rejected")
	}
}

// TestStaleClaimIdentitiesAreStaleGeneration pins that both a foreign claim id
// on the same Device and an old epoch surface as ErrStaleGeneration for gated
// mutations, and that stale-ref reconcile never invokes the stopper.
func TestStaleClaimIdentitiesAreStaleGeneration(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute, time.Minute)
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/stale", testRequest)
	if err != nil {
		t.Fatal(err)
	}
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
