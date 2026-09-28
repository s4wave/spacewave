//go:build !tinygo && !goscript

package s4wave_forge_world

import (
	"context"
	"path"
	"slices"
	"strconv"
	"sync"

	"github.com/pkg/errors"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/world"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
)

// ErrWorkerStopPending reports a durable pending stop whose owner claim must
// keep renewing until the runtime stop confirms.
var ErrWorkerStopPending = errors.New("Worker runtime stop remains pending")

// WorkerAdmission owns one Worker's World capacity claim and Docker reservations.
type WorkerAdmission struct {
	// admission owns the durable capacity and reservation state.
	admission *forge_runtime.WorldRuntimeAdmission
	// workerKey identifies the Worker served by this plugin resource.
	workerKey string
	// claimID identifies this Worker execution lifetime.
	claimID string

	// launchMtx serializes Docker launch with policy drain and close.
	launchMtx sync.Mutex
	// mtx guards the claim and admission fence without covering Docker create.
	mtx sync.Mutex
	// ref is the current Device and invocation claim.
	ref forge_runtime.WorkerClaimRef
	// epoch is the current durable owner epoch.
	epoch uint64
	// active permits new Docker reservations.
	active bool
	// changed closes when active admission is fenced for a policy transition.
	changed chan struct{}
	// policy is the last declaration to restore after pending stops finish.
	policy *device_policy.ForgeWorkerPolicy
}

// NewWorkerAdmission constructs one Worker admission with a Docker stopper.
func NewWorkerAdmission(eng world.Engine, workerKey, claimID string, stopper forge_runtime.RuntimeStopper) *WorkerAdmission {
	return &WorkerAdmission{
		admission: forge_runtime.NewWorldRuntimeAdmission(eng, stopper, 0, 0),
		workerKey: workerKey,
		claimID:   claimID,
		changed:   make(chan struct{}),
	}
}

// ApplyPolicy claims and observes this Worker or drains it when policy removes capacity.
func (w *WorkerAdmission) ApplyPolicy(ctx context.Context, deviceKey string, declared *device_policy.ForgeWorkerPolicy) error {
	w.mtx.Lock()
	if w.active {
		w.active = false
		close(w.changed)
		w.changed = make(chan struct{})
	}
	w.mtx.Unlock()
	w.launchMtx.Lock()
	defer w.launchMtx.Unlock()
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if deviceKey == "" {
		deviceKey = w.ref.DeviceObjectKey
	}
	if deviceKey == "" && declared == nil {
		return nil
	}
	if deviceKey == "" {
		return errors.New("enrolled Device identity is required for Worker capacity")
	}
	firstClaim := w.epoch == 0
	w.ref = forge_runtime.WorkerClaimRef{DeviceObjectKey: deviceKey, ClaimID: w.claimID}
	w.policy = nil
	if declared != nil {
		w.policy = declared.CloneVT()
	}

	// Reclaim the durable record before either observing or draining it.
	if declared == nil || declared.GetWorkerObjectKey() != w.workerKey {
		if _, err := w.admission.LookupWorkerCapacityAdmission(ctx, w.workerKey); errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
			return nil
		} else if err != nil {
			return err
		}
	}
	capacity, err := w.admission.ClaimWorkerCapacity(ctx, w.workerKey, w.ref)
	if err != nil {
		return err
	}
	w.epoch = capacity.OwnerEpoch
	if capacity.OwnerState == forge_runtime.CapacityOwnerStateDraining {
		ref, epoch := w.ref, w.epoch
		w.mtx.Unlock()
		_, err := w.admission.ReconcilePendingStops(ctx, ref)
		if err == nil {
			err = w.admission.StopWorkerReservations(ctx, w.workerKey, ref, epoch)
		}
		w.mtx.Lock()
		if err != nil {
			return errors.Wrap(ErrWorkerStopPending, err.Error())
		}
	}
	if firstClaim && declared != nil && declared.GetWorkerObjectKey() == w.workerKey {
		if _, err := w.admission.BeginDrainCapacity(ctx, w.workerKey, w.ref, w.epoch); err != nil {
			return err
		}
		ref, epoch := w.ref, w.epoch
		w.mtx.Unlock()
		err := w.admission.StopWorkerReservations(ctx, w.workerKey, ref, epoch)
		w.mtx.Lock()
		if err != nil {
			return errors.Wrap(ErrWorkerStopPending, err.Error())
		}
		capacity, err = w.admission.LookupWorkerCapacityAdmission(ctx, w.workerKey)
		if err != nil {
			return err
		}
		if capacity.MilliCPUReserved != 0 || capacity.MemoryBytesReserved != 0 {
			return ErrWorkerStopPending
		}
	}

	// A removed Docker backend must stop existing Docker custody before the
	// remaining declaration can reopen admission.
	if declared != nil && declared.GetWorkerObjectKey() == w.workerKey && !slices.Contains(declared.GetBackends(), "docker") {
		if _, err := w.admission.BeginDrainCapacity(ctx, w.workerKey, w.ref, w.epoch); err != nil {
			return err
		}
		ref, epoch := w.ref, w.epoch
		w.mtx.Unlock()
		err := w.admission.StopWorkerReservations(ctx, w.workerKey, ref, epoch)
		w.mtx.Lock()
		if err != nil {
			return errors.Wrap(ErrWorkerStopPending, err.Error())
		}
	}

	// An absent or retargeted declaration must stop every persisted runtime.
	if declared == nil || declared.GetWorkerObjectKey() != w.workerKey {
		ref, epoch := w.ref, w.epoch
		w.mtx.Unlock()
		err := w.drain(ctx, ref, epoch)
		w.mtx.Lock()
		if err != nil {
			return errors.Wrap(ErrWorkerStopPending, err.Error())
		}
		w.epoch = 0
		return nil
	}
	capacity, err = w.admission.ObserveWorker(ctx, w.workerKey, w.ref, w.epoch,
		declared.GetMilliCpu(), declared.GetMemoryBytes(), declared.GetBackends())
	if err != nil {
		return err
	}
	w.active = capacity.OwnerState == forge_runtime.CapacityOwnerStateActive
	if !w.active {
		ref, epoch := w.ref, w.epoch
		w.mtx.Unlock()
		err := w.admission.StopWorkerReservations(ctx, w.workerKey, ref, epoch)
		w.mtx.Lock()
		if err != nil {
			return errors.Wrap(ErrWorkerStopPending, err.Error())
		}
		capacity, err = w.admission.LookupWorkerCapacityAdmission(ctx, w.workerKey)
		if err != nil {
			return err
		}
		w.active = capacity.OwnerState == forge_runtime.CapacityOwnerStateActive
		if !w.active {
			return ErrWorkerStopPending
		}
	}
	return nil
}

// Renew extends the live Worker claim while this execution owns capacity.
func (w *WorkerAdmission) Renew(ctx context.Context) error {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.epoch == 0 {
		return nil
	}
	capacity, err := w.admission.RenewWorkerClaim(ctx, w.workerKey, w.ref)
	if err != nil {
		return err
	}
	w.epoch = capacity.OwnerEpoch
	if err := w.admission.RenewWorkerReservations(ctx, w.workerKey, w.ref); err != nil {
		return err
	}
	if capacity.OwnerState != forge_runtime.CapacityOwnerStateDraining {
		return nil
	}
	if !w.launchMtx.TryLock() {
		return nil
	}
	defer w.launchMtx.Unlock()
	stopCtx, cancel := context.WithTimeout(ctx, forge_runtime.DefaultOwnerLeaseDuration/3)
	defer cancel()
	if _, err := w.admission.ReconcilePendingStops(stopCtx, w.ref); err != nil {
		return errors.Wrap(ErrWorkerStopPending, err.Error())
	}
	if w.policy == nil || w.policy.GetWorkerObjectKey() != w.workerKey {
		ref, epoch := w.ref, w.epoch
		w.mtx.Unlock()
		err := w.drain(stopCtx, ref, epoch)
		w.mtx.Lock()
		if err != nil {
			return errors.Wrap(ErrWorkerStopPending, err.Error())
		}
		w.epoch = 0
		return nil
	}
	if err := w.admission.StopWorkerReservations(stopCtx, w.workerKey, w.ref, w.epoch); err != nil {
		return errors.Wrap(ErrWorkerStopPending, err.Error())
	}
	capacity, err = w.admission.ObserveWorker(ctx, w.workerKey, w.ref, w.epoch,
		w.policy.GetMilliCpu(), w.policy.GetMemoryBytes(), w.policy.GetBackends())
	if err != nil {
		return err
	}
	w.active = capacity.OwnerState == forge_runtime.CapacityOwnerStateActive
	if !w.active {
		return ErrWorkerStopPending
	}
	return nil
}

// Close drains this Worker's remaining runtimes on a clean execution exit.
// A crashed process leaves the claim to expire at its lease deadline.
func (w *WorkerAdmission) Close(ctx context.Context) error {
	w.mtx.Lock()
	w.active = false
	w.mtx.Unlock()
	w.launchMtx.Lock()
	defer w.launchMtx.Unlock()
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if w.ref.DeviceObjectKey == "" {
		return nil
	}
	if _, err := w.admission.LookupWorkerCapacityAdmission(ctx, w.workerKey); errors.Is(err, forge_runtime.ErrWorkerNotObserved) {
		return nil
	} else if err != nil {
		return err
	}
	ref, epoch := w.ref, w.epoch
	w.mtx.Unlock()
	err := w.drain(ctx, ref, epoch)
	w.mtx.Lock()
	if err != nil {
		return err
	}
	w.epoch = 0
	return nil
}

// drain stops the Worker's runtimes before removing its claim, allowing
// renewal until stop custody has been confirmed.
func (w *WorkerAdmission) drain(ctx context.Context, ref forge_runtime.WorkerClaimRef, epoch uint64) error {
	if _, err := w.admission.BeginDrainCapacity(ctx, w.workerKey, ref, epoch); err != nil {
		return err
	}
	if err := w.admission.StopWorkerReservations(ctx, w.workerKey, ref, epoch); err != nil {
		return err
	}
	return w.admission.CompleteDrainCapacity(ctx, w.workerKey, ref, epoch)
}

// Reserve debits the Docker target's explicit request under the current claim.
func (w *WorkerAdmission) Reserve(ctx context.Context, executionKey string, conf *forge_lib_docker.Config) (forge_lib_docker.Reservation, error) {
	w.mtx.Lock()
	defer w.mtx.Unlock()
	if !w.active {
		return nil, forge_runtime.ErrCapacityDraining
	}
	request := forge_runtime.ResourceRequest{MilliCPU: conf.GetMilliCpu(), MemoryBytes: conf.GetMemoryBytes(), Backend: "docker"}
	res, err := w.admission.ReserveForClaim(ctx, w.workerKey, executionKey, request, w.ref, w.epoch)
	if err != nil {
		return nil, err
	}
	name := "spacewave-" + path.Base(res.ObjectKey()) + "-" + strconv.FormatUint(res.Generation, 10)
	return &workerDockerReservation{worker: w, key: res.ObjectKey(), generation: res.Generation, name: name, conf: conf.CloneVT()}, nil
}

// workerDockerReservation couples a durable reservation to one named runtime.
type workerDockerReservation struct {
	worker     *WorkerAdmission
	key        string
	generation uint64
	name       string
	conf       *forge_lib_docker.Config
}

// Launch records stop custody and holds the launch fence through create and start.
func (r *workerDockerReservation) Launch(ctx context.Context, createAndStart func(string) error) error {
	w := r.worker
	w.launchMtx.Lock()
	defer w.launchMtx.Unlock()
	w.mtx.Lock()
	if !w.active {
		w.mtx.Unlock()
		return forge_runtime.ErrCapacityDraining
	}
	command := r.conf.GetDockerPath()
	if command == "" {
		command = "docker"
	}
	identity := forge_runtime.BackendRuntimeIdentity{
		Backend: "docker", ID: r.name, StopCommand: command,
		StopEnv: forge_lib_docker.BuildDockerEnv(r.conf), StopTimeoutSeconds: r.conf.GetStopTimeoutSeconds(),
	}
	if _, err := w.admission.ActivateForClaim(ctx, r.key, identity, w.ref, w.epoch); err != nil {
		w.mtx.Unlock()
		return err
	}
	w.mtx.Unlock()
	return createAndStart(r.name)
}

// Release stops the named runtime and credits its reservation once.
func (r *workerDockerReservation) Release(ctx context.Context) error {
	w := r.worker
	w.mtx.Lock()
	defer w.mtx.Unlock()
	_, err := w.admission.StopAndRelease(ctx, w.ref, w.epoch, r.key, r.generation)
	return err
}

// _ is a type assertion.
var _ forge_lib_docker.Admission = (*WorkerAdmission)(nil)
