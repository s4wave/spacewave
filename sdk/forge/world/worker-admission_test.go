//go:build !tinygo

package s4wave_forge_world

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/block"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/sirupsen/logrus"
)

// admissionStopper records durable runtime stops at the Worker boundary.
type admissionStopper struct {
	// stopped records each Docker stop attempt.
	stopped []forge_runtime.BackendRuntimeIdentity
	// failOnce returns one Docker stop error.
	failOnce bool
	// unconfirmedStops counts unconfirmed stops without an error.
	unconfirmedStops int
	// entered signals that Docker stop has begun.
	entered chan struct{}
	// release lets a blocked Docker stop confirm.
	release chan struct{}
}

// StopRuntime confirms a stop and records the persisted runtime name.
func (s *admissionStopper) StopRuntime(ctx context.Context, rt forge_runtime.BackendRuntimeIdentity) (bool, error) {
	// Record the runtime stop and wait on any configured stop gate.
	s.stopped = append(s.stopped, rt)
	if s.entered != nil {
		close(s.entered)
		select {
		case <-s.release:
		case <-ctx.Done():
			return false, ctx.Err()
		}
	}

	// Return the configured error or unconfirmed runtime stop result.
	if s.failOnce {
		s.failOnce = false
		return false, errors.New("Docker stop unconfirmed")
	}
	if s.unconfirmedStops != 0 {
		s.unconfirmedStops--
		return false, nil
	}
	return true, nil
}

// newWorkerAdmissionTestbed opens an offline World with a declared Docker Worker.
func newWorkerAdmissionTestbed(t *testing.T) (*WorkerAdmission, *admissionStopper, world.Engine) {
	// Open the storage testbed for Worker capacity records.
	t.Helper()
	ctx := t.Context()
	btb, err := hydra_testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(btb.Release)

	// Open the offline World on the storage testbed.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	// Claim the declared Docker Worker with a recording stopper.
	stopper := &admissionStopper{}
	admission := NewWorkerAdmission(wtb.Engine, "worker/a", wtb.Volume.GetPeerID(), "claim-1", stopper)
	if err := admission.ApplyPolicy(ctx, "devices/self", &device_policy.ForgeWorkerPolicy{
		WorkerObjectKey: "worker/a", MilliCpu: 2000, MemoryBytes: 2 << 30, Backends: []string{"docker"},
	}); err != nil {
		t.Fatal(err)
	}
	return admission, stopper, wtb.Engine
}

// reserveDocker creates a real Execution for the admission boundary to inspect.
func reserveDocker(t *testing.T, admission *WorkerAdmission, ctx context.Context, key string, conf *forge_lib_docker.Config) (forge_lib_docker.Reservation, error) {
	// Create the Execution that the Worker reservation will inspect.
	t.Helper()
	ws := world.NewEngineWorldState(admission.engine, true)
	_, err := forge_execution.CreateExecutionWithTarget(ctx, ws, admission.peerID, key, admission.peerID,
		forge_target.NewValueSet(), &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}, nil, timestamp.Now())
	if err != nil {
		t.Fatal(err)
	}
	return admission.Reserve(ctx, key, conf)
}

// TestWorkerAdmissionRejectsOtherWorkerPlacement keeps backend creation behind
// the selected Worker's capacity claim even if a resolver picks this factory.
func TestWorkerAdmissionRejectsOtherWorkerPlacement(t *testing.T) {
	// Create a Worker admission and Execution assigned to another Worker.
	admission, _, _ := newWorkerAdmissionTestbed(t)
	ctx := t.Context()
	const key = "exec/other-worker"
	ws := world.NewEngineWorldState(admission.engine, true)
	object, _, err := world.CreateWorldObject(ctx, ws, key, func(cursor *block.Cursor) error {
		cursor.SetBlock(&forge_execution.Execution{
			ExecutionState: forge_execution.State_ExecutionState_PENDING,
			PeerId:         admission.peerID.String(),
			Placement:      &forge_worker.Placement{WorkerObjectKey: "worker/b", PeerId: admission.peerID.String()},
			Timestamp:      timestamp.Now(),
		}, true)
		return nil
	})
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Worker refuses capacity for another Worker placement.
	if _, err := admission.Reserve(ctx, key, &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20}); err == nil {
		t.Fatal("backend capacity was reserved on another Worker")
	}
}

// TestWorkerAdmissionReleaseStopsAndCredits proves one active runtime stops
// before its debit is credited, and a repeated release has no extra effect.
func TestWorkerAdmissionReleaseStopsAndCredits(t *testing.T) {
	// Reserve Docker capacity in an offline Worker World.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	ctx := t.Context()
	grant, err := reserveDocker(t, admission, ctx, "exec/one", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}

	// Read the durable reservation request.
	reserved, err := admission.admission.LookupReservation(ctx, forge_runtime.BuildReservationObjectKey("exec/one"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the requested Docker resources reached the reservation.
	if reserved.Request.MilliCPU != 500 || reserved.Request.MemoryBytes != 1<<20 || reserved.Request.Backend != "docker" {
		t.Fatalf("Docker request did not reach Reserve: %+v", reserved.Request)
	}

	// Launch the runtime and read its debited Worker capacity.
	if err := grant.Launch(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	before, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the active runtime holds its capacity debit.
	if before.MilliCPUReserved != 500 || before.MemoryBytesReserved != 1<<20 {
		t.Fatalf("reservation debit missing: %+v", before)
	}

	// Release the runtime twice to exercise terminal cleanup.
	if err := grant.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if err := grant.Release(ctx); err != nil {
		t.Fatal(err)
	}

	// Read the Worker capacity after runtime cleanup.
	after, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify cleanup credits capacity and stops the runtime exactly once.
	if after.MilliCPUReserved != 0 || after.MemoryBytesReserved != 0 {
		t.Fatalf("reservation debit retained: %+v", after)
	}
	if len(stopper.stopped) != 1 || stopper.stopped[0].ID == "" {
		t.Fatalf("runtime stop count = %+v", stopper.stopped)
	}
}

// TestWorkerAdmissionPolicyRemovalDrainsRunningRuntime proves policy removal
// stops a persisted runtime and deletes the drained capacity record.
func TestWorkerAdmissionPolicyRemovalDrainsRunningRuntime(t *testing.T) {
	// Reserve and launch a runtime on the declared Docker Worker.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	ctx := t.Context()
	grant, err := reserveDocker(t, admission, ctx, "exec/drain", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := grant.Launch(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Remove the Worker policy to drain the runtime.
	if err := admission.ApplyPolicy(ctx, "devices/self", nil); err != nil {
		t.Fatal(err)
	}

	// Verify policy removal stopped the runtime and removed capacity.
	if len(stopper.stopped) != 1 {
		t.Fatalf("policy drain stopped %d runtimes, want 1", len(stopper.stopped))
	}
	if _, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a"); !errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
		t.Fatalf("drained capacity retained: %v", err)
	}

	// Verify releasing the drained reservation completes.
	if err := grant.Release(ctx); err != nil {
		t.Fatalf("terminal release: %v", err)
	}
}

// TestWorkerAdmissionPendingStopRetainsClaim proves a failed Docker stop keeps
// the debit and owner until a lease renewal confirms stop and completes drain.
func TestWorkerAdmissionPendingStopRetainsClaim(t *testing.T) {
	// Reserve and launch a runtime with a bounded test context.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	grant, err := reserveDocker(t, admission, ctx, "exec/pending", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := grant.Launch(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Remove policy while the Docker stop reports an error.
	stopper.failOnce = true
	if err := admission.ApplyPolicy(ctx, "devices/self", nil); !errors.Is(err, ErrWorkerStopPending) {
		t.Fatalf("unconfirmed policy drain = %v, want pending stop", err)
	}

	// Read the capacity retained by the unconfirmed stop.
	capacity, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the pending stop retains its claim and capacity debit.
	if capacity.MilliCPUReserved != 500 || capacity.OwnerState != forge_runtime.CapacityOwnerStateDraining || admission.epoch == 0 {
		t.Fatalf("unconfirmed stop lost owner or debit: %+v", capacity)
	}

	// Read the reservation awaiting runtime stop confirmation.
	key := forge_runtime.BuildReservationObjectKey("exec/pending")
	res, err := admission.admission.LookupReservation(ctx, key)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the unconfirmed reservation remains debited.
	if res.State != forge_runtime.ReservationStatePendingStop || (res.Cleanup != nil && res.Cleanup.CapacityReleased) {
		t.Fatalf("unconfirmed stop was credited: %+v", res)
	}

	// Renew the claim to reconcile the pending runtime stop.
	if err := admission.Renew(ctx); err != nil {
		t.Fatalf("pending stop renewal = %v", err)
	}

	// Read the reservation after stop reconciliation.
	res, err = admission.admission.LookupReservation(ctx, key)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the confirmed stop credits capacity and removes the drained Worker.
	if !res.State.Terminal() || !res.Cleanup.RuntimeStopped || !res.Cleanup.CapacityReleased {
		t.Fatalf("confirmed stop did not credit: %+v", res)
	}
	if _, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a"); !errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
		t.Fatalf("drained capacity retained: %v", err)
	}

	// Verify renewal after drain completes without another runtime stop.
	if err := admission.Renew(ctx); err != nil {
		t.Fatalf("renew after drain = %v", err)
	}
	if len(stopper.stopped) != 2 {
		t.Fatalf("runtime stop attempts = %d, want 2", len(stopper.stopped))
	}
}

// TestWorkerAdmissionUnconfirmedStopRetainsClaim covers a stopper that reports
// no error but cannot yet confirm that Docker stopped the runtime.
func TestWorkerAdmissionUnconfirmedStopRetainsClaim(t *testing.T) {
	// Reserve and launch a runtime with a bounded test context.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	grant, err := reserveDocker(t, admission, ctx, "exec/unconfirmed", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := grant.Launch(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Remove Worker policy while runtime stop remains unconfirmed.
	stopper.unconfirmedStops = 2
	if err := admission.ApplyPolicy(ctx, "devices/self", nil); !errors.Is(err, ErrWorkerStopPending) {
		t.Fatalf("unconfirmed policy drain = %v", err)
	}

	// Renew the Worker claim to finish the pending stop.
	if err := admission.Renew(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify confirmed stop removes Worker capacity after three attempts.
	if _, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a"); !errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
		t.Fatalf("drained capacity retained: %v", err)
	}
	if len(stopper.stopped) != 3 {
		t.Fatalf("runtime stop attempts = %d, want 3", len(stopper.stopped))
	}
}

// TestWorkerAdmissionPolicyReductionStopsAndCredits proves a smaller envelope
// stops an active Docker runtime before reopening admission.
func TestWorkerAdmissionPolicyReductionStopsAndCredits(t *testing.T) {
	// Reserve and launch a runtime under the original Worker capacity.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	ctx := t.Context()
	grant, err := reserveDocker(t, admission, ctx, "exec/reduced", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := grant.Launch(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Reduce the declared Worker capacity below the active reservation.
	if err := admission.ApplyPolicy(ctx, "devices/self", &device_policy.ForgeWorkerPolicy{
		WorkerObjectKey: "worker/a", MilliCpu: 200, MemoryBytes: 1 << 20, Backends: []string{"docker"},
	}); err != nil {
		t.Fatal(err)
	}

	// Read the Worker capacity after policy reduction.
	capacity, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the smaller policy stopped the runtime and reopened admission.
	if capacity.MilliCPUReserved != 0 || capacity.OwnerState != forge_runtime.CapacityOwnerStateActive {
		t.Fatalf("lowered policy retained debit or drain: %+v", capacity)
	}
	if len(stopper.stopped) != 1 {
		t.Fatalf("lowered policy stopped %d runtimes, want 1", len(stopper.stopped))
	}

	// Verify renewal and release complete after policy reduction.
	if err := admission.Renew(ctx); err != nil {
		t.Fatal(err)
	}
	if err := grant.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestWorkerAdmissionBackendRemovalStopsAndCredits proves a fitting envelope
// cannot preserve Docker custody after removing its backend capability.
func TestWorkerAdmissionBackendRemovalStopsAndCredits(t *testing.T) {
	// Reserve and launch a Docker runtime under the original policy.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	ctx := t.Context()
	grant, err := reserveDocker(t, admission, ctx, "exec/backend-removal", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := grant.Launch(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Replace Docker with the native Worker backend.
	if err := admission.ApplyPolicy(ctx, "devices/self", &device_policy.ForgeWorkerPolicy{
		WorkerObjectKey: "worker/a", MilliCpu: 2000, MemoryBytes: 2 << 30, Backends: []string{"native"},
	}); err != nil {
		t.Fatal(err)
	}

	// Read the capacity after removing the Docker backend.
	capacity, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify backend removal stops Docker and rejects new Docker reservations.
	if capacity.MilliCPUReserved != 0 || capacity.MemoryBytesReserved != 0 || capacity.OwnerState != forge_runtime.CapacityOwnerStateActive {
		t.Fatalf("backend removal retained Docker debit: %+v", capacity)
	}
	if len(stopper.stopped) != 1 {
		t.Fatalf("backend removal stopped %d runtimes, want 1", len(stopper.stopped))
	}
	if _, err := reserveDocker(t, admission, ctx, "exec/rejected", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20}); !errors.Is(err, forge_runtime.ErrBackendUnsupported) {
		t.Fatalf("Docker reservation after backend removal = %v", err)
	}

	// Release the terminal Docker reservation.
	if err := grant.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestWorkerAdmissionBackendRemovalWaitsForStop keeps the Worker draining
// when Docker cannot yet confirm its stop under a fitting new envelope.
func TestWorkerAdmissionBackendRemovalWaitsForStop(t *testing.T) {
	// Reserve and launch a Docker runtime before changing backends.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	ctx := t.Context()
	grant, err := reserveDocker(t, admission, ctx, "exec/backend-pending", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := grant.Launch(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Remove Docker while runtime stop remains unconfirmed.
	stopper.unconfirmedStops = 2
	policy := &device_policy.ForgeWorkerPolicy{
		WorkerObjectKey: "worker/a", MilliCpu: 2000, MemoryBytes: 2 << 30, Backends: []string{"native"},
	}
	if err := admission.ApplyPolicy(ctx, "devices/self", policy); !errors.Is(err, ErrWorkerStopPending) {
		t.Fatalf("unconfirmed Docker stop = %v, want pending", err)
	}

	// Read the Worker capacity while Docker stop is pending.
	capacity, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the unconfirmed stop keeps admission draining and debited.
	if capacity.OwnerState != forge_runtime.CapacityOwnerStateDraining || capacity.MilliCPUReserved != 500 {
		t.Fatalf("unconfirmed stop reopened admission: %+v", capacity)
	}

	// Renew the Worker claim to reconcile pending Docker cleanup.
	if err := admission.Renew(ctx); err != nil {
		t.Fatal(err)
	}

	// Read Worker capacity after runtime stop confirmation.
	capacity, err = admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify confirmed cleanup reopens capacity without the Docker debit.
	if capacity.OwnerState != forge_runtime.CapacityOwnerStateActive || capacity.MilliCPUReserved != 0 {
		t.Fatalf("confirmed stop did not reopen policy: %+v", capacity)
	}

	// Release the terminal Docker reservation.
	if err := grant.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestWorkerAdmissionBlockedLaunchRenewsAndDrains proves renewal can run while
// Docker create is blocked and policy removal cannot leave a late container.
func TestWorkerAdmissionBlockedLaunchRenewsAndDrains(t *testing.T) {
	// Reserve the active and late Docker launches with a bounded context.
	admission, stopper, _ := newWorkerAdmissionTestbed(t)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	grant, err := reserveDocker(t, admission, ctx, "exec/blocked", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	late, err := reserveDocker(t, admission, ctx, "exec/late", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}

	// Start Docker creation behind a controlled callback gate.
	created := make(chan struct{})
	continueCreate := make(chan struct{})
	launchDone := make(chan error, 1)
	go func() {
		launchDone <- grant.Launch(ctx, func(string) error {
			close(created)
			<-continueCreate
			return nil
		})
	}()
	select {
	case <-created:
	case <-time.After(5 * time.Second):
		t.Fatal("Docker create did not block")
	}

	// Advance the admission clock past the original owner deadline and prove
	// the live launch can extend both claim and reservation custody.
	var now atomic.Int64
	base := time.Now().UTC()
	now.Store(base.Add(40 * time.Second).UnixNano())
	admission.admission.SetTimeNow(func() time.Time { return time.Unix(0, now.Load()).UTC() })
	if err := admission.Renew(ctx); err != nil {
		t.Fatal(err)
	}

	// Advance the controlled clock and read the renewed Worker claim.
	now.Store(base.Add(65 * time.Second).UnixNano())
	capacity, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Worker claim survives beyond its original deadline.
	if !time.Unix(0, now.Load()).Before(capacity.OwnerLeaseExpiresAt.AsTime()) {
		t.Fatalf("owner claim expired during create: %v", capacity.OwnerLeaseExpiresAt.AsTime())
	}

	// Read the runtime reservation after claim renewal.
	res, err := admission.admission.LookupReservation(ctx, forge_runtime.BuildReservationObjectKey("exec/blocked"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify the runtime reservation lease also advanced.
	if !time.Unix(0, now.Load()).Before(res.LeaseExpiresAt.AsTime()) {
		t.Fatalf("reservation expired during create: %v", res.LeaseExpiresAt.AsTime())
	}
	if !res.LeaseExpiresAt.AsTime().After(base.Add(forge_runtime.DefaultLeaseDuration)) {
		t.Fatalf("reservation lease was not renewed during create: %v", res.LeaseExpiresAt.AsTime())
	}

	// Remove policy while create is blocked. Drain must wait for the callback
	// and then stop its named runtime before releasing the capacity record.
	drainDone := make(chan error, 1)
	drainEntered := make(chan struct{})
	admission.mtx.Lock()
	fenced := admission.changed
	admission.mtx.Unlock()
	go func() {
		close(drainEntered)
		drainDone <- admission.ApplyPolicy(ctx, "devices/self", nil)
	}()
	<-drainEntered

	// Wait for policy removal to fence Worker admission.
	select {
	case <-fenced:
	case <-ctx.Done():
		t.Fatal("policy removal did not fence admission")
	}

	// Verify policy drain remains blocked on Docker creation.
	select {
	case err := <-drainDone:
		t.Fatalf("policy drain passed blocked create: %v", err)
	default:
	}

	// Inspect Worker admission while policy drain is pending.
	admission.mtx.Lock()
	active := admission.active
	admission.mtx.Unlock()

	// Verify the pending policy drain fenced new reservations.
	if active {
		t.Fatal("policy removal did not fence admission during blocked create")
	}

	// Renew the pending drain while Docker creation remains blocked.
	now.Store(base.Add(90 * time.Second).UnixNano())
	if err := admission.Renew(ctx); err != nil {
		t.Fatalf("renew during pending drain: %v", err)
	}

	// Release Docker creation and wait for launch and drain completion.
	close(continueCreate)
	if err := <-launchDone; err != nil {
		t.Fatal(err)
	}
	if err := <-drainDone; err != nil {
		t.Fatal(err)
	}

	// Verify drain stopped the runtime and rejects a late launch.
	if len(stopper.stopped) != 1 {
		t.Fatalf("drain stopped %d runtimes, want 1", len(stopper.stopped))
	}
	started := false
	if err := late.Launch(ctx, func(string) error { started = true; return nil }); !errors.Is(err, forge_runtime.ErrCapacityDraining) {
		t.Fatalf("late launch error = %v, want draining", err)
	}
	if started {
		t.Fatal("late Docker runtime started after drain")
	}
}

// TestWorkerAdmissionLaunchFailureStopsNamedRuntime proves create or start
// failure leaves the preactivated Docker name under cleanup custody.
func TestWorkerAdmissionLaunchFailureStopsNamedRuntime(t *testing.T) {
	for _, stage := range []string{"create", "start"} {
		t.Run(stage, func(t *testing.T) {
			// Reserve Docker capacity for the failing launch stage.
			admission, stopper, _ := newWorkerAdmissionTestbed(t)
			ctx := t.Context()
			grant, err := reserveDocker(t, admission, ctx, "exec/failed", &forge_lib_docker.Config{
				Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20,
			})
			if err != nil {
				t.Fatal(err)
			}

			// Fail the runtime launch after capturing its durable name.
			var name string
			launchErr := errors.New(stage + " failed")
			if err := grant.Launch(ctx, func(runtimeName string) error {
				name = runtimeName
				return launchErr
			}); !errors.Is(err, launchErr) {
				t.Fatalf("Launch error = %v, want %v", err, launchErr)
			}

			// Release the failed launch to stop the named runtime.
			if err := grant.Release(ctx); err != nil {
				t.Fatal(err)
			}

			// Verify cleanup stops the exact runtime name once.
			if len(stopper.stopped) != 1 || stopper.stopped[0].ID != name || name == "" {
				t.Fatalf("named runtime cleanup = %+v, want %q", stopper.stopped, name)
			}

			// Read Worker capacity after failed runtime cleanup.
			capacity, err := admission.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
			if err != nil {
				t.Fatal(err)
			}

			// Verify the failed launch retains no capacity debit.
			if capacity.MilliCPUReserved != 0 || capacity.MemoryBytesReserved != 0 {
				t.Fatalf("failed launch retained debit: %+v", capacity)
			}
		})
	}
}

// TestWorkerAdmissionRenewExtendsActiveReservation keeps a long running
// Docker target from losing its lease while its Worker remains alive.
func TestWorkerAdmissionRenewExtendsActiveReservation(t *testing.T) {
	// Reserve Docker capacity for a long-running execution.
	admission, _, _ := newWorkerAdmissionTestbed(t)
	ctx := t.Context()
	grant, err := reserveDocker(t, admission, ctx, "exec/long", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}

	// Advance the controlled clock and renew the Worker claim.
	now := time.Now().UTC().Add(30 * time.Second)
	admission.admission.SetTimeNow(func() time.Time { return now })
	if err := admission.Renew(ctx); err != nil {
		t.Fatal(err)
	}

	// Read the active reservation after lease renewal.
	res, err := admission.admission.LookupReservation(ctx, forge_runtime.BuildReservationObjectKey("exec/long"))
	if err != nil {
		t.Fatal(err)
	}

	// Verify renewal extends the runtime lease.
	if res.LeaseExpiresAt.AsTime().Before(now.Add(forge_runtime.DefaultLeaseDuration)) {
		t.Fatalf("reservation lease did not advance: %v", res.LeaseExpiresAt.AsTime())
	}

	// Release the renewed runtime reservation.
	if err := grant.Release(ctx); err != nil {
		t.Fatal(err)
	}
}

// TestWorkerAdmissionReclaimStopsPreviousRuntime proves a restarted Worker
// reclaims the Device claim and stops old custody before admitting new work.
func TestWorkerAdmissionReclaimStopsPreviousRuntime(t *testing.T) {
	// Reserve and launch a runtime under the original Worker claim.
	admission, stopper, eng := newWorkerAdmissionTestbed(t)
	ctx := t.Context()
	grant, err := reserveDocker(t, admission, ctx, "exec/old", &forge_lib_docker.Config{Image: "img", MilliCpu: 500, MemoryBytes: 1 << 20})
	if err != nil {
		t.Fatal(err)
	}
	if err := grant.Launch(ctx, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}

	// Reclaim the Worker capacity with a replacement execution.
	replacement := NewWorkerAdmission(eng, "worker/a", admission.peerID, "claim-2", stopper)
	if err := replacement.ApplyPolicy(ctx, "devices/self", &device_policy.ForgeWorkerPolicy{
		WorkerObjectKey: "worker/a", MilliCpu: 2000, MemoryBytes: 2 << 30, Backends: []string{"docker"},
	}); err != nil {
		t.Fatal(err)
	}

	// Verify reclaim stops the previous runtime once.
	if len(stopper.stopped) != 1 {
		t.Fatalf("reclaim stopped %d runtimes, want 1", len(stopper.stopped))
	}

	// Read the replacement Worker capacity claim.
	capacity, err := replacement.admission.LookupWorkerCapacityAdmission(ctx, "worker/a")
	if err != nil {
		t.Fatal(err)
	}

	// Verify reclaim advances the epoch and clears the stale debit.
	if capacity.MilliCPUReserved != 0 || capacity.OwnerEpoch <= 1 {
		t.Fatalf("reclaim retained stale debit or epoch: %+v", capacity)
	}

	// Verify late release of the previous reservation completes.
	if err := grant.Release(ctx); err != nil {
		t.Fatalf("late terminal release: %v", err)
	}
}
