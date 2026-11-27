package forge_runtime

import (
	"context"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	"github.com/sirupsen/logrus"
)

type testStopper struct {
	mtx     sync.Mutex
	stopped map[string]int
	failFor string
	stopErr error
	calls   atomic.Int64
}

func newTestStopper() *testStopper {
	return &testStopper{stopped: make(map[string]int)}
}

func (s *testStopper) StopRuntime(_ context.Context, rt *BackendRuntimeIdentity) (bool, error) {
	// Count runtime stop calls under the test stopper lock.
	s.calls.Add(1)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if rt.Id == s.failFor {
		return false, s.stopErr
	}

	// Record the confirmed stop for this runtime identity.
	s.stopped[rt.Id]++
	return true, nil
}

func (s *testStopper) count(id string) int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.stopped[id]
}

func newTestbed(t *testing.T) (context.Context, world.Engine, *world_testbed.Testbed) {
	// Start the block-store testbed and register its cleanup.
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { btb.Release() })

	// Start the World engine over the block-store testbed.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wtb.Release() })
	return ctx, wtb.Engine, wtb
}

type staleOnceEngine struct {
	world.Engine
	stale atomic.Bool
}

func (e *staleOnceEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil || !write || e.stale.Swap(false) == false {
		return tx, err
	}
	return &staleOnceWorldState{Tx: tx}, nil
}

type staleOnceWorldState struct {
	world.Tx
}

func (s *staleOnceWorldState) Commit(context.Context) error {
	return coord.ErrStaleGeneration
}

func TestWorldRuntimeAdmissionRetriesStaleWorldRoot(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	wrapped := &staleOnceEngine{Engine: eng}
	wrapped.stale.Store(true)
	admission := NewWorldRuntimeAdmission(wrapped, nil, 0, 0)
	attempts := 0
	if err := admission.withTx(ctx, true, func(context.Context, world.WorldState) error {
		attempts++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if attempts != 2 {
		t.Fatalf("transaction attempts = %d, want 2", attempts)
	}
}

var testRequest = &ResourceRequest{MilliCpu: 1_000, MemoryBytes: 1 << 30, Backend: "docker"}

func TestReserveActivateStopRoundTrip(t *testing.T) {
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
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker", "v86"}, nil); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != ReservationState_RESERVATION_STATE_RESERVED || res.Generation != 1 || res.ObjectKey() != BuildReservationObjectKey("exec/1") {
		t.Fatalf("unexpected reservation: %+v", res)
	}

	// Idempotent reserve for the same attempt proves the debit.
	reused, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil || reused.Generation != res.Generation {
		t.Fatalf("expected idempotent reserve: %+v err=%v", reused, err)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err := LookupWorkerCapacityEngine(ctx, eng, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != 1_000 || capacity.MemoryBytesReserved != 1<<30 {
		t.Fatalf("capacity not debited: %+v", capacity)
	}

	// Activate the reservation with a backend runtime identity.
	rt := &BackendRuntimeIdentity{Backend: "docker", Id: "container-1"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// Read the durable reservation and check its custody state.
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationState_RESERVATION_STATE_ACTIVE || !reflect.DeepEqual(loaded.Runtime, rt) {
		t.Fatalf("unexpected active reservation: %+v", loaded)
	}
	if loaded.Outcome(time.Now()) != OutcomeActive {
		t.Fatalf("expected active outcome, got %d", loaded.Outcome(time.Now()))
	}

	// Stop the backend runtime and check the cleanup receipt.
	receipt, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Complete() || receipt.RuntimeIdentity != "container-1" || receipt.Reason != CleanupReason_CLEANUP_REASON_STOPPED {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	if stopper.count("container-1") != 1 {
		t.Fatalf("runtime stopper called %d times", stopper.count("container-1"))
	}

	// Idempotent stop returns the persisted receipt without re-stopping.
	receipt2, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1)
	if err != nil || receipt2 == nil || !receipt2.Complete() {
		t.Fatalf("expected persisted receipt on retry: %+v err=%v", receipt2, err)
	}
	if stopper.count("container-1") != 1 {
		t.Fatal("stopper re-invoked after release")
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err = LookupWorkerCapacityEngine(ctx, eng, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != 0 || capacity.MemoryBytesReserved != 0 {
		t.Fatalf("capacity not credited: %+v", capacity)
	}

	// The resumed Execution reserves the next generation and debits again.
	resumed, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Generation != res.Generation+1 || resumed.State != ReservationState_RESERVATION_STATE_RESERVED {
		t.Fatalf("expected next reserved generation: %+v", resumed)
	}
	capacity, err = LookupWorkerCapacityEngine(ctx, eng, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != testRequest.MilliCpu || capacity.MemoryBytesReserved != testRequest.MemoryBytes {
		t.Fatalf("resumed reservation not debited: %+v", capacity)
	}

	// The old generation is fenced from the resumed reservation.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), res.Generation); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected stale generation, got %v", err)
	}
}

func TestReserveConcurrentOversubscribeRejected(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 2<<30, []string{"docker"}, nil); err != nil {
		t.Fatal(err)
	}

	// Launch concurrent reservations against the same Worker capacity.
	const attempts = 8
	var wg sync.WaitGroup
	var successes atomic.Int64
	errs := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			// Attempt one reservation and record its result for this caller.
			defer wg.Done()
			req := &ResourceRequest{MilliCpu: 1_000, MemoryBytes: 1 << 30, Backend: "docker"}
			_, err := admission.Reserve(ctx, "worker/a", "exec/concurrent-"+string(rune('a'+i)), req)
			if err == nil {
				successes.Add(1)
			} else {
				errs[i] = err
			}
		}(i)
	}
	wg.Wait()

	// Check that concurrent reservations respect the Worker capacity limit.
	if successes.Load() != 2 {
		t.Fatalf("expected exactly 2 successful reservations, got %d", successes.Load())
	}
	for i, err := range errs {
		if err != nil && !errors.Is(err, ErrCapacityExhausted) {
			t.Fatalf("attempt %d: unexpected error %v", i, err)
		}
	}

	// Read the durable Worker capacity after concurrent reservations.
	ws := world.NewEngineWorldState(eng, false)
	capacity, err := LookupWorkerCapacity(ctx, ws, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != 2_000 || capacity.MemoryBytesReserved != 2<<30 {
		t.Fatalf("reserved counts wrong: %+v", capacity)
	}
}

func TestRestartReconcileResumesWithoutRelaunch(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Hour, time.Hour)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 4_000, 8<<30, []string{"docker"}, nil); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/restart", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate the reservation with a backend runtime identity.
	rt := &BackendRuntimeIdentity{Backend: "docker", Id: "container-restart"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// A fresh admission over the same durable engine reconciles by lookup.
	restarted := NewWorldRuntimeAdmission(eng, stopper, time.Hour, time.Hour)

	// Read the durable reservation and check its custody state.
	loaded, err := restarted.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationState_RESERVATION_STATE_ACTIVE || !reflect.DeepEqual(loaded.Runtime, rt) || loaded.Generation != 1 {
		t.Fatalf("reconcile lost custody facts: %+v", loaded)
	}
	if loaded.Outcome(time.Now()) != OutcomeActive {
		t.Fatalf("expected active outcome after restart, got %d", loaded.Outcome(time.Now()))
	}
}

func TestUncertainRetainsDebitAndResumeRefencesGeneration(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Hour, time.Hour)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}, nil); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/uncertain", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate the reservation with a backend runtime identity.
	rt := &BackendRuntimeIdentity{Backend: "docker", Id: "container-u1"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// Mark runtime custody uncertain while its capacity debit remains held.
	if _, err := admission.MarkUncertain(ctx, res.ObjectKey()); err != nil {
		t.Fatal(err)
	}

	// Read the durable reservation and check its custody state.
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Outcome(time.Now()) != OutcomeUncertain {
		t.Fatalf("expected uncertain outcome, got %d", loaded.Outcome(time.Now()))
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != 1_000 {
		t.Fatal("uncertain disconnect must retain the debit")
	}

	// The same fenced runtime reconnects: generation re-fences upward.
	rt2 := &BackendRuntimeIdentity{Backend: "docker", Id: "container-u2"}
	if _, err := admission.ResumeFromUncertain(ctx, res.ObjectKey(), rt2); err != nil {
		t.Fatal(err)
	}

	// Late return from the replaced runtime instance is stale and inert.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected stale generation error, got %v", err)
	}
	if stopper.count("container-u1") != 0 && stopper.count("container-u2") != 0 {
		t.Fatal("stale late-return must not stop any runtime")
	}
	if stopper.calls.Load() != 0 {
		t.Fatal("stale late-return invoked the stopper")
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, _ = lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if capacity.MilliCpuReserved != 1_000 {
		t.Fatal("stale late-return must not credit capacity")
	}

	// Stop the backend runtime and check the cleanup receipt.
	receipt, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Generation != 2 || receipt.RuntimeIdentity != "container-u2" || !receipt.Complete() {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	if stopper.count("container-u2") != 1 {
		t.Fatal("current runtime not stopped")
	}
}

func TestExpiryFencesGenerationReleasesOnceAndStopsLateReturn(t *testing.T) {
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
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}, nil); err != nil {
		t.Fatal(err)
	}

	// Choose the time used to evaluate the Worker or reservation lease.
	now := time.Now().UTC().Truncate(time.Second)
	admission.SetTimeNow(func() time.Time { return now })

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/expiry", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate the reservation with a backend runtime identity.
	rt := &BackendRuntimeIdentity{Backend: "docker", Id: "container-exp"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// Not yet expired: expiry is a no-op.
	receipts, err := admission.ExpireLeases(ctx, selfRef, now.Add(30*time.Second))
	if err != nil || len(receipts) != 0 {
		t.Fatalf("expected no expiries: %+v err=%v", receipts, err)
	}

	// Choose the time used to evaluate the Worker or reservation lease.
	expiredAt := now.Add(2 * time.Minute)
	receipts, err = admission.ExpireLeases(ctx, selfRef, expiredAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("expected one expiry receipt, got %+v", receipts)
	}
	r := receipts[0]

	// The sweep fenced custody, confirmed the stop outside the Worker lock,
	// then credited the debit exactly once in the finalizing transaction.
	if r.Generation != 2 || r.Reason != CleanupReason_CLEANUP_REASON_EXPIRED || !r.Complete() {
		t.Fatalf("unexpected expiry receipt: %+v", r)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != 0 {
		t.Fatalf("confirmed expiry must credit the debit exactly once: %+v", capacity)
	}

	// Post-expiry old-runtime calls are stale: the fence moved.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected stale generation after expiry, got %v", err)
	}
	if stopper.count("container-exp") != 1 {
		t.Fatalf("stale post-expiry call re-invoked the stopper: %d", stopper.count("container-exp"))
	}

	// Read the durable reservation and check its custody state.
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Generation != 2 || loaded.State != ReservationState_RESERVATION_STATE_RELEASED || !loaded.Cleanup.Complete() {
		t.Fatalf("unexpected post-expiry record: %+v", loaded)
	}
	if loaded.Outcome(expiredAt) != OutcomeTerminal {
		t.Fatalf("released reservation reconciles as terminal, got %d", loaded.Outcome(expiredAt))
	}

	// Nothing remains pending for reconciliation.
	done, err := admission.ReconcilePendingStops(ctx, selfRef)
	if err != nil || len(done) != 0 {
		t.Fatalf("expected no pending stops after a confirmed sweep: %+v err=%v", done, err)
	}
	final, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 2)
	if err != nil || final == nil || !final.Complete() {
		t.Fatalf("expected terminal-idempotent receipt for fencing generation: %+v err=%v", final, err)
	}
	if stopper.count("container-exp") != 1 {
		t.Fatal("terminal-idempotent return re-invoked the stopper")
	}

	// Retry is a new attempt with a new Execution key.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/expiry-retry", testRequest); err != nil {
		t.Fatal(err)
	}
}

func TestExpiryWithFailingStopperKeepsPendingStopForReconcile(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	stopper.failFor = "container-flaky"
	stopper.stopErr = errors.New("docker daemon unreachable")
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 1_000, 1<<30, []string{"docker"}, nil); err != nil {
		t.Fatal(err)
	}

	// Choose the time used to evaluate the Worker or reservation lease.
	now := time.Now().UTC()
	admission.SetTimeNow(func() time.Time { return now })

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/flaky", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate runtime custody for the reserved Execution.
	if _, err := admission.Activate(ctx, res.ObjectKey(), &BackendRuntimeIdentity{Backend: "docker", Id: "container-flaky"}); err != nil {
		t.Fatal(err)
	}

	// Sweep expired reservation leases and check the cleanup receipts.
	receipts, err := admission.ExpireLeases(ctx, selfRef, now.Add(2*time.Minute))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("expected expiry receipt despite failed stop: %+v err=%v", receipts, err)
	}

	// Read the durable reservation and check its custody state.
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationState_RESERVATION_STATE_PENDING_STOP || loaded.Cleanup == nil ||
		loaded.Cleanup.RuntimeStopped || loaded.Cleanup.CapacityReleased {
		t.Fatalf("partial receipt not durable: %+v", loaded)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != testRequest.MilliCpu {
		t.Fatalf("debit must stay held while the stop is unconfirmed: %+v", capacity)
	}

	// Graceful stop of the flaky runtime also fails while it is unreachable.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 2); err == nil {
		t.Fatal("expected stop failure to surface")
	}

	// The daemon recovers. Reconciliation finishes the pending stop once.
	stopper.failFor = ""
	done, err := admission.ReconcilePendingStops(ctx, selfRef)
	if err != nil || len(done) != 1 || !done[0].Complete() {
		t.Fatalf("expected completed receipt: %+v err=%v", done, err)
	}

	// Read the durable reservation and check its custody state.
	final, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if final.State != ReservationState_RESERVATION_STATE_RELEASED || !final.Cleanup.Complete() {
		t.Fatalf("reservation not finalized: %+v", final)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err = lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != 0 {
		t.Fatalf("capacity credited twice or retained: %+v", capacity)
	}
}

func TestStopperErrorDuringGracefulStopKeepsReservationLive(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	stopper.failFor = "container-graceful"
	stopper.stopErr = errors.New("stop timeout")
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Hour, time.Hour)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 1_000, 1<<30, []string{"docker"}, nil); err != nil {
		t.Fatal(err)
	}

	// Reserve Worker capacity for the Execution attempt.
	res, err := admission.Reserve(ctx, "worker/a", "exec/graceful", testRequest)
	if err != nil {
		t.Fatal(err)
	}

	// Activate runtime custody for the reserved Execution.
	if _, err := admission.Activate(ctx, res.ObjectKey(), &BackendRuntimeIdentity{Backend: "docker", Id: "container-graceful"}); err != nil {
		t.Fatal(err)
	}

	// Check the runtime stop against the reservation and Worker fences.
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1); err == nil {
		t.Fatal("expected stopper error to surface")
	}

	// Read the durable reservation and check its custody state.
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationState_RESERVATION_STATE_PENDING_STOP {
		t.Fatalf("expected durable pending-stop after failed stop, got %d", loaded.State)
	}

	// Read the Worker capacity and check its reserved totals or lifecycle state.
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCpuReserved != 1_000 {
		t.Fatal("capacity must stay debited until the stop confirms")
	}

	// The stopper recovers. A retry with the same fenced generation
	// completes.
	stopper.failFor = ""

	// Stop the backend runtime and check the cleanup receipt.
	receipt, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), 1)
	if err != nil || !receipt.Complete() {
		t.Fatalf("expected completed receipt: %+v err=%v", receipt, err)
	}
}

func TestObserveWorkerPreservesDebitsAndRejectsUnknownBackend(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)

	// Claim the Worker capacity for the test Device.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 2_000, 4<<30, []string{"docker"}, nil); err != nil {
		t.Fatal(err)
	}

	// Reserve capacity for a new Execution attempt.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/backend", testRequest); err != nil {
		t.Fatal(err)
	}

	// Update Worker totals and check that reserved capacity survives.
	updated, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 4_000, 8<<30, []string{"docker"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if updated.MilliCpuReserved != 1_000 || updated.MilliCpuTotal != 4_000 {
		t.Fatalf("observation clobbered debits: %+v", updated)
	}

	// Reject a reservation for an undeclared Worker backend.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/v86", &ResourceRequest{MilliCpu: 100, MemoryBytes: 100, Backend: "v86"}); !errors.Is(err, ErrBackendUnsupported) {
		t.Fatalf("expected unsupported backend error, got %v", err)
	}

	// Reject reservations for a Worker without a capacity record.
	if _, err := admission.Reserve(ctx, "worker/unobserved", "exec/x", testRequest); !errors.Is(err, ErrWorkerNotObserved) {
		t.Fatalf("expected unobserved worker error, got %v", err)
	}
}

// lookupCapacityViaAdmission reads capacity through the admission's engine.
func lookupCapacityViaAdmission(ctx context.Context, admission *WorldRuntimeAdmission, workerObjectKey string) (*WorkerCapacity, error) {
	var out *WorkerCapacity
	err := admission.withTx(ctx, false, func(ctx context.Context, ws world.WorldState) error {
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		out = capacity
		return err
	})
	return out, err
}

// LookupWorkerCapacityEngine reads one Worker capacity record through an engine.
func LookupWorkerCapacityEngine(ctx context.Context, eng world.Engine, workerObjectKey string) (*WorkerCapacity, error) {
	var out *WorkerCapacity
	err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		out = capacity
		return err
	})
	return out, err
}

// TestReserveDrainingRejectionOnIdempotentPath proves the fail-before
// ordering: a draining capacity record rejects Reserve on the fresh path and
// on the idempotent existing path before any debit-proof read.
func TestReserveDrainingRejectionOnIdempotentPath(t *testing.T) {
	// Start a World testbed and admission service for the Worker.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	ref := WorkerClaimRef{DeviceObjectKey: "devices/self", ClaimID: "claim-1"}

	// Claim the Worker capacity for the test Device.
	capacity, err := admission.ClaimWorkerCapacity(ctx, "worker/a", ref)
	if err != nil {
		t.Fatal(err)
	}

	// Publish the Worker resource totals for the test scenario.
	if _, err := admission.ObserveWorker(ctx, "worker/a", ref, capacity.OwnerEpoch, 2_000, 4<<30, []string{"docker"}, nil); err != nil {
		t.Fatal(err)
	}

	// Reserve capacity for a new Execution attempt.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest); err != nil {
		t.Fatal(err)
	}

	// Move the Worker capacity into draining.
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", ref, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}

	// Reject reservations while the Worker capacity is draining.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/2", testRequest); !errors.Is(err, ErrCapacityDraining) {
		t.Fatalf("expected draining rejection on fresh path, got %v", err)
	}

	// Reject reservations while the Worker capacity is draining.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest); !errors.Is(err, ErrCapacityDraining) {
		t.Fatalf("expected draining rejection on idempotent path, got %v", err)
	}
}

// rawBody is a stored body written byte for byte, to seed records in the
// JSON form older releases wrote.
type rawBody []byte

// MarshalVT returns the body.
func (b *rawBody) MarshalVT() ([]byte, error) { return *b, nil }

// UnmarshalVT stores the body.
func (b *rawBody) UnmarshalVT(data []byte) error {
	*b = append((*b)[:0], data...)
	return nil
}

// TestLegacyJSONRecordsCarryAcross verifies that a Worker whose capacity and
// reservation records an older release wrote as JSON keeps its claim, its
// debit and its runtime custody, and that the next write stores protobuf.
func TestLegacyJSONRecordsCarryAcross(t *testing.T) {
	// Seed a claimed capacity record and an active reservation as JSON.
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute, time.Minute)
	lease, err := timestamp.New(time.Now().Add(time.Hour)).MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	capacityJSON := rawBody(`{"workerObjectKey":"worker/a","ownerDeviceObjectKey":"devices/self","claimId":"claim-1",` +
		`"ownerEpoch":5,"ownerLeaseExpiresAt":` + string(lease) + `,"ownerState":1,"milliCpuTotal":2000,` +
		`"milliCpuReserved":1000,"memoryBytesTotal":4294967296,"memoryBytesReserved":1073741824,` +
		`"backends":["docker"],"observedAt":` + string(lease) + `,"generation":3}`)
	resKey := BuildReservationObjectKey("exec/legacy")
	reservationJSON := rawBody(`{"workerObjectKey":"worker/a","executionObjectKey":"exec/legacy",` +
		`"request":{"milliCpu":1000,"memoryBytes":1073741824,"backend":"docker"},"generation":1,` +
		`"leaseExpiresAt":` + string(lease) + `,"state":2,"runtime":{"backend":"docker","id":"container-1",` +
		`"stopCommand":"docker","stopEnv":["A=1"],"stopTimeoutSeconds":10}}`)
	if err := admission.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		for key, body := range map[string]*rawBody{
			BuildWorkerCapacityObjectKey("worker/a"): &capacityJSON,
			resKey:                                   &reservationJSON,
		} {
			if _, _, err := world.AccessWorldObject(ctx, ws, key, true, func(bcs *block.Cursor) error {
				bcs.SetBlock(body, true)
				return nil
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// The same claim renews without a new epoch and keeps the debit.
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	if claimed.OwnerEpoch != 5 || claimed.MilliCpuReserved != 1000 || claimed.OwnerState != CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE {
		t.Fatalf("legacy claim not carried: %v", claimed)
	}

	// The reservation keeps its runtime custody.
	res, err := admission.LookupReservation(ctx, resKey)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != ReservationState_RESERVATION_STATE_ACTIVE || res.GetRuntime().GetId() != "container-1" || res.GetRuntime().GetStopTimeoutSeconds() != 10 {
		t.Fatalf("legacy reservation not carried: %v", res)
	}

	// Stopping the runtime credits the debit and stores both records as protobuf.
	receipt, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, resKey, res.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Complete() || receipt.Reason != CleanupReason_CLEANUP_REASON_STOPPED {
		t.Fatalf("receipt = %v", receipt)
	}
	if err := admission.withTx(ctx, false, func(ctx context.Context, ws world.WorldState) error {
		for _, key := range []string{BuildWorkerCapacityObjectKey("worker/a"), resKey} {
			data, _, err := world.LookupObjectBodyBytes(ctx, ws, key)
			if err != nil {
				return err
			}
			if len(data) == 0 || data[0] == '{' {
				t.Errorf("%s still stored as JSON", key)
			}
		}
		capacity, err := LookupWorkerCapacity(ctx, ws, "worker/a")
		if err != nil {
			return err
		}
		if capacity.MilliCpuReserved != 0 || capacity.MemoryBytesReserved != 0 {
			t.Errorf("debit not credited: %v", capacity)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestCapacityCardValidation verifies the card ledger rules: an ACTIVE record
// reserves only cards it offers and never one twice, and a DRAINING record may
// hold a card that left the host until its reservation credits it.
func TestCapacityCardValidation(t *testing.T) {
	now := timestamp.New(time.Now())
	card := &s4wave_device.GraphicsCard{
		Id:          "0000:03:00.0",
		Vendor:      s4wave_device.GraphicsCardVendor_GRAPHICS_CARD_VENDOR_AMD,
		MemoryBytes: 12_868_124_672,
		DevicePaths: []string{"/dev/kfd", "/dev/dri/renderD128"},
	}
	record := func(state CapacityOwnerState, cards []*s4wave_device.GraphicsCard, reserved ...string) *WorkerCapacity {
		return &WorkerCapacity{
			WorkerObjectKey: "worker/a", Generation: 1, ObservedAt: now,
			OwnerDeviceObjectKey: "devices/self", ClaimId: "claim-1", OwnerEpoch: 1,
			OwnerLeaseExpiresAt: now, OwnerState: state, Cards: cards, ReservedCardIds: reserved,
		}
	}
	active := CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE
	draining := CapacityOwnerState_CAPACITY_OWNER_STATE_DRAINING
	for name, tc := range map[string]struct {
		capacity *WorkerCapacity
		valid    bool
	}{
		"offered and reserved":     {record(active, []*s4wave_device.GraphicsCard{card}, card.Id), true},
		"reserved but not offered": {record(active, nil, card.Id), false},
		"reserved twice":           {record(active, []*s4wave_device.GraphicsCard{card}, card.Id, card.Id), false},
		"declared twice":           {record(active, []*s4wave_device.GraphicsCard{card, card}), false},
		"draining after removal":   {record(draining, nil, card.Id), true},
		"card without vendor":      {record(active, []*s4wave_device.GraphicsCard{{Id: card.Id, DevicePaths: card.DevicePaths}}), false},
	} {
		if err := tc.capacity.Validate(); (err == nil) != tc.valid {
			t.Errorf("%s: Validate() = %v, want valid %v", name, err, tc.valid)
		}
	}
}

// TestCardAdmission reserves a whole card, reports a busy card as exhausted
// and a request no offered card could meet as never fitting, and drains the
// record while a reserved card has left the host until its credit lands.
func TestCardAdmission(t *testing.T) {
	// Claim a Worker that offers one AMD card.
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute, time.Minute)
	claimed, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	card := &s4wave_device.GraphicsCard{
		Id:          "0000:03:00.0",
		Vendor:      s4wave_device.GraphicsCardVendor_GRAPHICS_CARD_VENDOR_AMD,
		MemoryBytes: 12 << 30,
		DevicePaths: []string{"/dev/kfd", "/dev/dri/renderD128"},
	}
	cards := []*s4wave_device.GraphicsCard{card}
	if _, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 4_000, 8<<30, []string{"docker"}, cards); err != nil {
		t.Fatal(err)
	}
	request := func(cards *s4wave_device.GraphicsCardRequest) *ResourceRequest {
		return &ResourceRequest{MilliCpu: 1_000, MemoryBytes: 1 << 30, Backend: "docker", Cards: cards}
	}
	amd := s4wave_device.GraphicsCardVendor_GRAPHICS_CARD_VENDOR_AMD

	// Reserve the card whole for the first Execution.
	res, err := admission.Reserve(ctx, "worker/a", "exec/a", request(&s4wave_device.GraphicsCardRequest{Count: 1, Vendor: amd, MinMemoryBytes: 8 << 30}))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.CardIds) != 1 || res.CardIds[0] != card.Id {
		t.Fatalf("reserved cards %v", res.CardIds)
	}

	// A second card request fits the Worker but waits for the held card.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/b", request(&s4wave_device.GraphicsCardRequest{Count: 1})); !errors.Is(err, ErrCapacityExhausted) {
		t.Fatalf("busy card: %v", err)
	}

	// Requests no offered card could meet are refused whatever is held.
	for name, cards := range map[string]*s4wave_device.GraphicsCardRequest{
		"two cards":   {Count: 2},
		"nvidia card": {Count: 1, Vendor: s4wave_device.GraphicsCardVendor_GRAPHICS_CARD_VENDOR_NVIDIA},
		"more memory": {Count: 1, MinMemoryBytes: 16 << 30},
	} {
		if _, err := admission.Reserve(ctx, "worker/a", "exec/"+name, request(cards)); !errors.Is(err, ErrCapacityNeverFits) {
			t.Fatalf("%s: %v", name, err)
		}
	}

	// The held card leaving the host drains the record until its credit.
	drained, err := admission.ObserveWorker(ctx, "worker/a", selfRef, claimed.OwnerEpoch, 4_000, 8<<30, []string{"docker"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if drained.OwnerState != CapacityOwnerState_CAPACITY_OWNER_STATE_DRAINING {
		t.Fatalf("held card removal left state %v", drained.OwnerState)
	}
	if _, err := admission.StopAndRelease(ctx, selfRef, claimed.OwnerEpoch, res.ObjectKey(), res.Generation); err != nil {
		t.Fatal(err)
	}
	credited, err := LookupWorkerCapacityEngine(ctx, eng, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if credited.OwnerState != CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE || len(credited.ReservedCardIds) != 0 {
		t.Fatalf("credit left %v holding %v", credited.OwnerState, credited.ReservedCardIds)
	}
}
