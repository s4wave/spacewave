package forge_runtime

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
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

func (s *testStopper) StopRuntime(_ context.Context, rt BackendRuntimeIdentity) (bool, error) {
	s.calls.Add(1)
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if rt.ID == s.failFor {
		return false, s.stopErr
	}
	s.stopped[rt.ID]++
	return true, nil
}

func (s *testStopper) count(id string) int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.stopped[id]
}

func newTestbed(t *testing.T) (context.Context, world.Engine, *world_testbed.Testbed) {
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { btb.Release() })
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { wtb.Release() })
	return ctx, wtb.Engine, wtb
}

var testRequest = ResourceRequest{MilliCPU: 1_000, MemoryBytes: 1 << 30, Backend: "docker"}

func TestReserveActivateStopRoundTrip(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute)

	if _, err := admission.ObserveWorker(ctx, "worker/a", 2_000, 4<<30, []string{"docker", "v86"}); err != nil {
		t.Fatal(err)
	}

	res, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if res.State != ReservationStateReserved || res.Generation != 1 || res.ObjectKey() != BuildReservationObjectKey("exec/1") {
		t.Fatalf("unexpected reservation: %+v", res)
	}

	// Idempotent reserve for the same attempt proves the debit.
	reused, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest)
	if err != nil || reused.Generation != res.Generation {
		t.Fatalf("expected idempotent reserve: %+v err=%v", reused, err)
	}
	capacity, err := LookupWorkerCapacityEngine(ctx, eng, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 1_000 || capacity.MemoryBytesReserved != 1<<30 {
		t.Fatalf("capacity not debited: %+v", capacity)
	}

	rt := BackendRuntimeIdentity{Backend: "docker", ID: "container-1"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationStateActive || loaded.Runtime != rt {
		t.Fatalf("unexpected active reservation: %+v", loaded)
	}
	if loaded.Outcome(time.Now()) != OutcomeActive {
		t.Fatalf("expected active outcome, got %d", loaded.Outcome(time.Now()))
	}

	receipt, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.Complete() || receipt.RuntimeIdentity != "container-1" || receipt.Reason != CleanupReasonStop {
		t.Fatalf("unexpected receipt: %+v", receipt)
	}
	if stopper.count("container-1") != 1 {
		t.Fatalf("runtime stopper called %d times", stopper.count("container-1"))
	}

	// Idempotent stop returns the persisted receipt without re-stopping.
	receipt2, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1)
	if err != nil || receipt2 == nil || !receipt2.Complete() {
		t.Fatalf("expected persisted receipt on retry: %+v err=%v", receipt2, err)
	}
	if stopper.count("container-1") != 1 {
		t.Fatal("stopper re-invoked after release")
	}

	capacity, err = LookupWorkerCapacityEngine(ctx, eng, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 0 || capacity.MemoryBytesReserved != 0 {
		t.Fatalf("capacity not credited: %+v", capacity)
	}

	// A retry after release is a new attempt with a new Execution key; the
	// released attempt rejects reuse.
	if _, err := admission.Reserve(ctx, "worker/a", "exec/1", testRequest); !errors.Is(err, ErrReservationTerminal) {
		t.Fatalf("expected terminal reservation error, got %v", err)
	}
}

func TestReserveConcurrentOversubscribeRejected(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 2_000, 2<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}

	const attempts = 8
	var wg sync.WaitGroup
	var successes atomic.Int64
	errs := make([]error, attempts)
	for i := range attempts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := ResourceRequest{MilliCPU: 1_000, MemoryBytes: 1 << 30, Backend: "docker"}
			_, err := admission.Reserve(ctx, "worker/a", "exec/concurrent-"+string(rune('a'+i)), req)
			if err == nil {
				successes.Add(1)
			} else {
				errs[i] = err
			}
		}(i)
	}
	wg.Wait()

	if successes.Load() != 2 {
		t.Fatalf("expected exactly 2 successful reservations, got %d", successes.Load())
	}
	for i, err := range errs {
		if err != nil && !errors.Is(err, ErrCapacityExhausted) {
			t.Fatalf("attempt %d: unexpected error %v", i, err)
		}
	}
	ws := world.NewEngineWorldState(eng, false)
	capacity, err := LookupWorkerCapacity(ctx, ws, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 2_000 || capacity.MemoryBytesReserved != 2<<30 {
		t.Fatalf("reserved counts wrong: %+v", capacity)
	}
}

func TestRestartReconcileResumesWithoutRelaunch(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Hour)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 4_000, 8<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/restart", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	rt := BackendRuntimeIdentity{Backend: "docker", ID: "container-restart"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// A fresh admission over the same durable engine reconciles by lookup.
	restarted := NewWorldRuntimeAdmission(eng, stopper, time.Hour)
	loaded, err := restarted.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationStateActive || loaded.Runtime != rt || loaded.Generation != 1 {
		t.Fatalf("reconcile lost custody facts: %+v", loaded)
	}
	if loaded.Outcome(time.Now()) != OutcomeActive {
		t.Fatalf("expected active outcome after restart, got %d", loaded.Outcome(time.Now()))
	}
}

func TestUncertainRetainsDebitAndResumeRefencesGeneration(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Hour)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/uncertain", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	rt := BackendRuntimeIdentity{Backend: "docker", ID: "container-u1"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	if _, err := admission.MarkUncertain(ctx, res.ObjectKey()); err != nil {
		t.Fatal(err)
	}
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Outcome(time.Now()) != OutcomeUncertain {
		t.Fatalf("expected uncertain outcome, got %d", loaded.Outcome(time.Now()))
	}
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 1_000 {
		t.Fatal("uncertain disconnect must retain the debit")
	}

	// The same fenced runtime reconnects: generation re-fences upward.
	rt2 := BackendRuntimeIdentity{Backend: "docker", ID: "container-u2"}
	if _, err := admission.ResumeFromUncertain(ctx, res.ObjectKey(), rt2); err != nil {
		t.Fatal(err)
	}

	// Late return from the replaced runtime instance is stale and inert.
	if _, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected stale generation error, got %v", err)
	}
	if stopper.count("container-u1") != 0 && stopper.count("container-u2") != 0 {
		t.Fatal("stale late-return must not stop any runtime")
	}
	if stopper.calls.Load() != 0 {
		t.Fatal("stale late-return invoked the stopper")
	}
	capacity, _ = lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if capacity.MilliCPUReserved != 1_000 {
		t.Fatal("stale late-return must not credit capacity")
	}

	receipt, err := admission.StopAndRelease(ctx, res.ObjectKey(), 2)
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
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	admission.SetTimeNow(func() time.Time { return now })

	res, err := admission.Reserve(ctx, "worker/a", "exec/expiry", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	rt := BackendRuntimeIdentity{Backend: "docker", ID: "container-exp"}
	if _, err := admission.Activate(ctx, res.ObjectKey(), rt); err != nil {
		t.Fatal(err)
	}

	// Not yet expired: expiry is a no-op.
	receipts, err := admission.ExpireLeases(ctx, now.Add(30*time.Second))
	if err != nil || len(receipts) != 0 {
		t.Fatalf("expected no expiries: %+v err=%v", receipts, err)
	}

	expiredAt := now.Add(2 * time.Minute)
	receipts, err = admission.ExpireLeases(ctx, expiredAt)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipts) != 1 {
		t.Fatalf("expected one expiry receipt, got %+v", receipts)
	}
	r := receipts[0]
	// The sweep fenced custody, confirmed the stop outside the Worker lock,
	// then credited the debit exactly once in the finalizing transaction.
	if r.Generation != 2 || r.Reason != CleanupReasonExpired || !r.Complete() {
		t.Fatalf("unexpected expiry receipt: %+v", r)
	}
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 0 {
		t.Fatalf("confirmed expiry must credit the debit exactly once: %+v", capacity)
	}

	// Post-expiry old-runtime calls are stale: the fence moved.
	if _, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("expected stale generation after expiry, got %v", err)
	}
	if stopper.count("container-exp") != 1 {
		t.Fatalf("stale post-expiry call re-invoked the stopper: %d", stopper.count("container-exp"))
	}
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Generation != 2 || loaded.State != ReservationStateReleased || !loaded.Cleanup.Complete() {
		t.Fatalf("unexpected post-expiry record: %+v", loaded)
	}
	if loaded.Outcome(expiredAt) != OutcomeTerminal {
		t.Fatalf("released reservation reconciles as terminal, got %d", loaded.Outcome(expiredAt))
	}

	// Nothing remains pending for reconciliation.
	done, err := admission.ReconcilePendingStops(ctx)
	if err != nil || len(done) != 0 {
		t.Fatalf("expected no pending stops after a confirmed sweep: %+v err=%v", done, err)
	}
	final, err := admission.StopAndRelease(ctx, res.ObjectKey(), 2)
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
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	stopper.failFor = "container-flaky"
	stopper.stopErr = errors.New("docker daemon unreachable")
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Minute)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 1_000, 1<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	admission.SetTimeNow(func() time.Time { return now })
	res, err := admission.Reserve(ctx, "worker/a", "exec/flaky", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.Activate(ctx, res.ObjectKey(), BackendRuntimeIdentity{Backend: "docker", ID: "container-flaky"}); err != nil {
		t.Fatal(err)
	}

	receipts, err := admission.ExpireLeases(ctx, now.Add(2*time.Minute))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("expected expiry receipt despite failed stop: %+v err=%v", receipts, err)
	}
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationStatePendingStop || loaded.Cleanup == nil ||
		loaded.Cleanup.RuntimeStopped || loaded.Cleanup.CapacityReleased {
		t.Fatalf("partial receipt not durable: %+v", loaded)
	}
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != testRequest.MilliCPU {
		t.Fatalf("debit must stay held while the stop is unconfirmed: %+v", capacity)
	}

	// Graceful stop of the flaky runtime also fails while it is unreachable.
	if _, err := admission.StopAndRelease(ctx, res.ObjectKey(), 2); err == nil {
		t.Fatal("expected stop failure to surface")
	}

	// The daemon recovers; reconciliation finishes the pending stop once.
	stopper.failFor = ""
	done, err := admission.ReconcilePendingStops(ctx)
	if err != nil || len(done) != 1 || !done[0].Complete() {
		t.Fatalf("expected completed receipt: %+v err=%v", done, err)
	}
	final, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if final.State != ReservationStateReleased || !final.Cleanup.Complete() {
		t.Fatalf("reservation not finalized: %+v", final)
	}
	capacity, err = lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 0 {
		t.Fatalf("capacity credited twice or retained: %+v", capacity)
	}
}

func TestStopperErrorDuringGracefulStopKeepsReservationLive(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	stopper := newTestStopper()
	stopper.failFor = "container-graceful"
	stopper.stopErr = errors.New("stop timeout")
	admission := NewWorldRuntimeAdmission(eng, stopper, time.Hour)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 1_000, 1<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	res, err := admission.Reserve(ctx, "worker/a", "exec/graceful", testRequest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admission.Activate(ctx, res.ObjectKey(), BackendRuntimeIdentity{Backend: "docker", ID: "container-graceful"}); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1); err == nil {
		t.Fatal("expected stopper error to surface")
	}
	loaded, err := admission.LookupReservation(ctx, res.ObjectKey())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.State != ReservationStatePendingStop {
		t.Fatalf("expected durable pending-stop after failed stop, got %d", loaded.State)
	}
	capacity, err := lookupCapacityViaAdmission(ctx, admission, "worker/a")
	if err != nil {
		t.Fatal(err)
	}
	if capacity.MilliCPUReserved != 1_000 {
		t.Fatal("capacity must stay debited until the stop confirms")
	}

	// The stopper recovers; a retry with the same fenced generation completes.
	stopper.failFor = ""
	receipt, err := admission.StopAndRelease(ctx, res.ObjectKey(), 1)
	if err != nil || !receipt.Complete() {
		t.Fatalf("expected completed receipt: %+v err=%v", receipt, err)
	}
}

func TestObserveWorkerPreservesDebitsAndRejectsUnknownBackend(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.Reserve(ctx, "worker/a", "exec/backend", testRequest); err != nil {
		t.Fatal(err)
	}
	updated, err := admission.ObserveWorker(ctx, "worker/a", 4_000, 8<<30, []string{"docker"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.MilliCPUReserved != 1_000 || updated.MilliCPUTotal != 4_000 {
		t.Fatalf("observation clobbered debits: %+v", updated)
	}
	if _, err := admission.Reserve(ctx, "worker/a", "exec/v86", ResourceRequest{MilliCPU: 100, MemoryBytes: 100, Backend: "v86"}); !errors.Is(err, ErrBackendUnsupported) {
		t.Fatalf("expected unsupported backend error, got %v", err)
	}
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
