package forge_runtime

import (
	"context"
	"sync"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
)

// RuntimeStopper stops one backend runtime by identity. The Forge runtime
// backend owns the mechanics; admission owns the fence and the ledger.
// Implementations must be idempotent: stopping an already-gone runtime
// returns true, nil.
type RuntimeStopper interface {
	// StopRuntime stops the identified runtime.
	// Returns true when the runtime was stopped or was already gone.
	StopRuntime(ctx context.Context, rt BackendRuntimeIdentity) (bool, error)
}

// WorldRuntimeAdmission implements RuntimeAdmission over durable world objects.
//
// Every reservation and capacity mutation applies inside one world write
// transaction, so create-plus-debit and release-plus-credit are atomic even
// across a crash. All mutations for one Worker serialize on a per-Worker lock;
// the durable boundary assumes exactly one writer instance per Forge Worker
// capacity record. A second process writing the same Worker's capacity record
// is outside this contract and must not be attempted: run one admission owner
// per Worker.
type WorldRuntimeAdmission struct {
	// eng is the world engine holding the durable state.
	eng world.Engine
	// stopper stops backend runtimes during release and reconciliation.
	stopper RuntimeStopper
	// lease is the reservation lease duration.
	lease time.Duration
	// ownerLease is the owner claim lease duration.
	ownerLease time.Duration
	// now returns the current time; overridable in tests.
	now func() time.Time

	mtx sync.Mutex
	// workerLocks serialize capacity mutations per Worker object key.
	workerLocks map[string]*sync.Mutex
}

// NewWorldRuntimeAdmission constructs a world-backed RuntimeAdmission.
// A zero lease uses DefaultLeaseDuration; a zero ownerLease uses
// DefaultOwnerLeaseDuration.
func NewWorldRuntimeAdmission(eng world.Engine, stopper RuntimeStopper, lease, ownerLease time.Duration) *WorldRuntimeAdmission {
	if lease == 0 {
		lease = DefaultLeaseDuration
	}
	if ownerLease == 0 {
		ownerLease = DefaultOwnerLeaseDuration
	}
	return &WorldRuntimeAdmission{
		eng:         eng,
		stopper:     stopper,
		lease:       lease,
		ownerLease:  ownerLease,
		now:         time.Now,
		workerLocks: make(map[string]*sync.Mutex),
	}
}

// SetTimeNow overrides the clock; tests use this to drive expiry.
func (a *WorldRuntimeAdmission) SetTimeNow(now func() time.Time) {
	a.now = now
}

// lockWorker locks the per-Worker mutation lock and returns the unlock func.
func (a *WorldRuntimeAdmission) lockWorker(workerObjectKey string) func() {
	// Find or create the Worker mutation lock under the admission mutex.
	a.mtx.Lock()
	lk, ok := a.workerLocks[workerObjectKey]
	if !ok {
		lk = &sync.Mutex{}
		a.workerLocks[workerObjectKey] = lk
	}
	a.mtx.Unlock()

	// Serialize mutations to this Worker capacity record.
	lk.Lock()
	return lk.Unlock
}

// withTx runs cb inside one world transaction of the given mode.
func (a *WorldRuntimeAdmission) withTx(
	ctx context.Context,
	write bool,
	cb func(ctx context.Context, ws world.WorldState) error,
) error {
	const maxStaleRetries = 8
	var err error
	for attempt := 0; attempt <= maxStaleRetries; attempt++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		err = world.ExecTransaction(ctx, a.eng, write, cb)
		if !write || !errors.Is(err, coord.ErrStaleGeneration) {
			return err
		}
	}
	return err
}

// loadOwnedCapacity loads the capacity record inside the transaction and
// verifies the caller's ref and epoch against its durable claim. The record
// must be owned by the same Device and claim with the current epoch and an
// unexpired lease. This is the in-transaction half of every gated mutation.
func loadOwnedCapacity(
	ctx context.Context,
	ws world.WorldState,
	workerObjectKey string,
	ref WorkerClaimRef,
	epoch uint64,
	now time.Time,
) (*WorkerCapacity, error) {
	// Load the Worker capacity record inside the current transaction.
	capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
	if err != nil {
		return nil, err
	}

	// Require a durable Worker claim before checking its identity.
	if !capacity.owned() {
		return nil, ErrCapacityUnowned
	}

	// Fence the Worker claim by Device, lease and epoch.
	expired := capacity.OwnerLeaseExpiresAt == nil || !now.Before(capacity.OwnerLeaseExpiresAt.AsTime())
	switch {
	case capacity.OwnerDeviceObjectKey != ref.DeviceObjectKey:
		if expired {
			return nil, ErrCapacityOwnerExpired
		}
		return nil, ErrCapacityOwned
	case expired:
		return nil, ErrCapacityOwnerExpired
	case capacity.ClaimID != ref.ClaimID || capacity.OwnerEpoch != epoch:
		return nil, ErrStaleGeneration
	}
	return capacity, nil
}

// verifyLiveClaim checks that the record's durable claim matches the caller's
// reference and is unexpired. Sweeps use this variant: they carry no epoch
// argument because the stored epoch is current for the live owner.
func verifyLiveClaim(capacity *WorkerCapacity, ref WorkerClaimRef, now time.Time) error {
	if !capacity.owned() {
		return ErrCapacityUnowned
	}
	if capacity.OwnerDeviceObjectKey != ref.DeviceObjectKey || capacity.ClaimID != ref.ClaimID {
		if capacity.OwnerLeaseExpiresAt == nil || !now.Before(capacity.OwnerLeaseExpiresAt.AsTime()) {
			return ErrCapacityOwnerExpired
		}
		return ErrCapacityOwned
	}
	if capacity.OwnerLeaseExpiresAt == nil || !now.Before(capacity.OwnerLeaseExpiresAt.AsTime()) {
		return ErrCapacityOwnerExpired
	}
	return nil
}

// ClaimWorkerCapacity claims or reclaims the Worker's capacity record for the
// calling instance. An absent record is created at epoch 1 with zero totals; a
// legacy ownerless record or an expired lease is reclaimed with an epoch bump
// that preserves OwnerState so resumed drains stay draining. A live foreign
// Device claim fails with ErrCapacityOwned. The same Device and claim id renew
// idempotently without bumping the epoch.
func (a *WorldRuntimeAdmission) ClaimWorkerCapacity(
	ctx context.Context,
	workerObjectKey string,
	ref WorkerClaimRef,
) (*WorkerCapacity, error) {
	// Require a Worker key and a complete Device claim.
	if workerObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "worker_object_key")
	}
	if ref.DeviceObjectKey == "" || ref.ClaimID == "" {
		return nil, errors.New("claim reference must be complete")
	}

	// Serialize the Worker claim transition.
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	// Persist the Worker claim and return its committed capacity.
	var out *WorkerCapacity
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Read the existing Worker claim, allowing an unobserved Worker.
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if err != nil && !errors.Is(err, ErrWorkerNotObserved) {
			return err
		}

		// Create, renew or reclaim the Worker capacity claim.
		now := a.now().UTC()
		if capacity == nil {
			capacity = &WorkerCapacity{
				WorkerObjectKey:      workerObjectKey,
				MilliCPUTotal:        0,
				MemoryBytesTotal:     0,
				ObservedAt:           timestamp.New(now),
				OwnerDeviceObjectKey: ref.DeviceObjectKey,
				ClaimID:              ref.ClaimID,
				OwnerEpoch:           1,
				OwnerLeaseExpiresAt:  timestamp.New(now.Add(a.ownerLease)),
				OwnerState:           CapacityOwnerStateActive,
			}
			capacity.Generation = 1
		} else {
			// Reject a foreign Device holding a live Worker claim.
			live := capacity.owned() &&
				capacity.OwnerLeaseExpiresAt != nil &&
				now.Before(capacity.OwnerLeaseExpiresAt.AsTime())
			if live && capacity.OwnerDeviceObjectKey != ref.DeviceObjectKey {
				return ErrCapacityOwned
			}

			// Renew the same Worker claim or replace its epoch.
			sameClaim := live &&
				capacity.OwnerDeviceObjectKey == ref.DeviceObjectKey &&
				capacity.ClaimID == ref.ClaimID
			if sameClaim {
				// Idempotent renew: extend the lease in place.
				capacity.OwnerLeaseExpiresAt = timestamp.New(now.Add(a.ownerLease))
			} else {
				// Reclaim: legacy ownerless, expired lease, or a new claim id
				// on the same Device. Preserve OwnerState and observed totals;
				// bump the epoch so stale instances fence out.
				prevEpoch := capacity.OwnerEpoch
				capacity.OwnerDeviceObjectKey = ref.DeviceObjectKey
				capacity.ClaimID = ref.ClaimID
				capacity.OwnerEpoch = prevEpoch + 1
				capacity.OwnerLeaseExpiresAt = timestamp.New(now.Add(a.ownerLease))
				if capacity.OwnerState == CapacityOwnerStateUnspecified {
					capacity.OwnerState = CapacityOwnerStateActive
				}
			}
		}

		// Validate the Worker identity and claim generation.
		if capacity.WorkerObjectKey == "" {
			capacity.WorkerObjectKey = workerObjectKey
		}
		capacity.Generation++
		if err := capacity.Validate(); err != nil {
			return err
		}

		// Save the Worker claim for the caller.
		if err := persistWorkerCapacity(ctx, ws, workerObjectKey, capacity); err != nil {
			return err
		}
		out = capacity
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RenewWorkerClaim extends the owner claim lease of one Worker. Renewal on an
// expired lease falls back to reclaim: the same ref reclaims with an epoch
// bump and preserved state instead of stalling its own sweeps.
func (a *WorldRuntimeAdmission) RenewWorkerClaim(
	ctx context.Context,
	workerObjectKey string,
	ref WorkerClaimRef,
) (*WorkerCapacity, error) {
	// Serialize renewal of the Worker claim.
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	// Renew the Worker claim in one durable transaction.
	var out *WorkerCapacity
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Load the Worker capacity record for renewal.
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if err != nil {
			return err
		}

		// Require the renewing Device and claim to match the durable record.
		now := a.now().UTC()
		if !capacity.owned() {
			return ErrCapacityUnowned
		}
		if capacity.OwnerDeviceObjectKey != ref.DeviceObjectKey {
			return ErrCapacityOwned
		}
		if capacity.ClaimID != ref.ClaimID {
			return ErrStaleGeneration
		}

		// Reclaim an expired Worker epoch and extend its lease.
		expired := capacity.OwnerLeaseExpiresAt == nil ||
			!now.Before(capacity.OwnerLeaseExpiresAt.AsTime())
		if expired {
			capacity.OwnerEpoch++
		}
		capacity.OwnerLeaseExpiresAt = timestamp.New(now.Add(a.ownerLease))
		capacity.Generation++
		if err := capacity.Validate(); err != nil {
			return err
		}

		// Persist the renewed Worker claim for the caller.
		if err := persistWorkerCapacity(ctx, ws, workerObjectKey, capacity); err != nil {
			return err
		}
		out = capacity
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ObserveWorker upserts the Worker's observed capacity totals and backends
// under a live owner claim at the given epoch. Reserved debits are preserved;
// every observation bumps the record generation and stamps ObservedAt.
// Declared totals below current debits move the record to DRAINING until
// credits land; fitting totals return it to ACTIVE. Empty backends are a
// validation error: only BeginDrainCapacity empties the backend list.
func (a *WorldRuntimeAdmission) ObserveWorker(
	ctx context.Context,
	workerObjectKey string,
	ref WorkerClaimRef,
	epoch uint64,
	milliCPUTotal, memoryBytesTotal uint64,
	backends []string,
) (*WorkerCapacity, error) {
	// Require a Worker key and at least one supported backend.
	if workerObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "worker_object_key")
	}
	if len(backends) == 0 {
		return nil, errors.New("backends must not be empty")
	}

	// Serialize changes to the Worker capacity totals.
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	// Persist the Worker observation under its live claim.
	var out *WorkerCapacity
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Fence the observation against the durable Worker claim.
		capacity, err := loadOwnedCapacity(ctx, ws, workerObjectKey, ref, epoch, a.now())
		if err != nil {
			return err
		}

		// Update Worker totals and determine whether reserved capacity still fits.
		capacity.MilliCPUTotal = milliCPUTotal
		capacity.MemoryBytesTotal = memoryBytesTotal
		capacity.Backends = append([]string(nil), backends...)
		fits := capacity.MilliCPUReserved <= capacity.MilliCPUTotal &&
			capacity.MemoryBytesReserved <= capacity.MemoryBytesTotal
		if fits {
			capacity.OwnerState = CapacityOwnerStateActive
		} else {
			capacity.OwnerState = CapacityOwnerStateDraining
		}

		// Stamp and validate the Worker observation.
		capacity.ObservedAt = timestamp.Now()
		capacity.Generation++
		if err := capacity.Validate(); err != nil {
			return err
		}

		// Persist the observed Worker capacity for the caller.
		if err := persistWorkerCapacity(ctx, ws, workerObjectKey, capacity); err != nil {
			return err
		}
		out = capacity
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// BeginDrainCapacity moves the record to DRAINING with empty backends under a
// live claim at the given epoch. Reserved debits stay held; sweeps continue.
// Idempotent.
func (a *WorldRuntimeAdmission) BeginDrainCapacity(
	ctx context.Context,
	workerObjectKey string,
	ref WorkerClaimRef,
	epoch uint64,
) (*WorkerCapacity, error) {
	// Serialize the Worker transition into draining.
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	// Persist the draining Worker capacity under its live claim.
	var out *WorkerCapacity
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Fence the drain against the durable Worker claim.
		capacity, err := loadOwnedCapacity(ctx, ws, workerObjectKey, ref, epoch, a.now())
		if err != nil {
			return err
		}

		// Block new reservations and validate the draining capacity.
		capacity.OwnerState = CapacityOwnerStateDraining
		capacity.Backends = nil
		capacity.Generation++
		if err := capacity.Validate(); err != nil {
			return err
		}

		// Save the draining Worker capacity for the caller.
		if err := persistWorkerCapacity(ctx, ws, workerObjectKey, capacity); err != nil {
			return err
		}
		out = capacity
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DrainWorker blocks new reservations, stops each live runtime, and removes
// the capacity record after every debit has been credited.
func (a *WorldRuntimeAdmission) DrainWorker(
	ctx context.Context,
	workerObjectKey string,
	ref WorkerClaimRef,
	epoch uint64,
) error {
	if _, err := a.BeginDrainCapacity(ctx, workerObjectKey, ref, epoch); err != nil {
		return err
	}
	if err := a.StopWorkerReservations(ctx, workerObjectKey, ref, epoch); err != nil {
		return err
	}
	return a.CompleteDrainCapacity(ctx, workerObjectKey, ref, epoch)
}

// StopWorkerReservations stops persisted runtimes from a previous Worker
// execution and reconciles interrupted stops under the current claim.
func (a *WorldRuntimeAdmission) StopWorkerReservations(
	ctx context.Context,
	workerObjectKey string,
	ref WorkerClaimRef,
	epoch uint64,
) error {
	// Stop persisted reservations even when their original controller is gone.
	keys, err := a.listReservationKeys(ctx)
	if err != nil {
		return err
	}
	for _, key := range keys {
		res, err := a.LookupReservation(ctx, key)
		if err != nil {
			return err
		}
		if res.WorkerObjectKey != workerObjectKey || res.State.Terminal() {
			continue
		}
		if res.State == ReservationStatePendingStop {
			continue
		}
		if _, err := a.StopAndRelease(ctx, ref, epoch, key, res.Generation); err != nil {
			return err
		}
	}
	if _, err := a.ReconcilePendingStops(ctx, ref); err != nil {
		return err
	}

	// A stopper may report no error without confirming the stop. Keep the
	// Worker draining until every reservation reaches a terminal receipt.
	for _, key := range keys {
		res, err := a.LookupReservation(ctx, key)
		if err != nil {
			return err
		}
		if res.WorkerObjectKey == workerObjectKey && !res.State.Terminal() {
			return errors.Errorf("worker %s still holds reservation %s", workerObjectKey, key)
		}
	}
	return nil
}

// CompleteDrainCapacity deletes a fully drained capacity record exactly once.
// It requires the DRAINING state and no non-terminal reservation referencing
// the Worker; the scan runs inside the same transaction as the deletion.
func (a *WorldRuntimeAdmission) CompleteDrainCapacity(
	ctx context.Context,
	workerObjectKey string,
	ref WorkerClaimRef,
	epoch uint64,
) error {
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	return a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Require a live Worker claim in the draining state.
		capacity, err := loadOwnedCapacity(ctx, ws, workerObjectKey, ref, epoch, a.now())
		if err != nil {
			return err
		}
		if capacity.OwnerState != CapacityOwnerStateDraining {
			return errors.New("capacity record is not draining")
		}

		// Check every reservation before deleting the Worker capacity.
		resKeys, err := listReservationKeys(ctx, ws)
		if err != nil {
			return err
		}
		for _, resKey := range resKeys {
			res, err := LookupReservation(ctx, ws, resKey)
			if errors.Is(err, ErrReservationNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if res.WorkerObjectKey == workerObjectKey && !res.State.Terminal() {
				return errors.Errorf("worker %s still holds active reservation %s",
					workerObjectKey, resKey)
			}
		}
		return deleteWorkerCapacity(ctx, ws, workerObjectKey)
	})
}

// ScanOwnedCapacity returns the capacity records owned by one Device, paired
// with their Worker object keys.
func (a *WorldRuntimeAdmission) ScanOwnedCapacity(
	ctx context.Context,
	deviceObjectKey string,
) ([]OwnedWorkerCapacity, error) {
	var out []OwnedWorkerCapacity
	err := a.withTx(ctx, false, func(ctx context.Context, ws world.WorldState) error {
		keys, err := listWorkerCapacityKeys(ctx, ws)
		if err != nil {
			return err
		}
		for _, objKey := range keys {
			capacity, err := world.LookupObjectBody[*WorkerCapacity](ctx, ws, objKey, NewWorkerCapacityBlock)
			if errors.Is(err, world.ErrObjectNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			if capacity.OwnerDeviceObjectKey != deviceObjectKey {
				continue
			}
			out = append(out, OwnedWorkerCapacity{
				WorkerObjectKey: capacity.WorkerObjectKey,
				Capacity:        capacity,
			})
		}
		return nil
	})
	return out, err
}

// Reserve implements RuntimeAdmission. Creation and the capacity debit apply
// in one transaction; the idempotent return proves the debit by re-reading the
// capacity record before returning.
func (a *WorldRuntimeAdmission) Reserve(
	ctx context.Context,
	workerObjectKey, executionObjectKey string,
	request ResourceRequest,
) (*Reservation, error) {
	return a.reserve(ctx, workerObjectKey, executionObjectKey, request, nil, 0)
}

// ReserveForClaim reserves capacity only while the caller owns the current
// Worker claim epoch. The claim check and debit share one World transaction.
func (a *WorldRuntimeAdmission) ReserveForClaim(
	ctx context.Context,
	workerObjectKey, executionObjectKey string,
	request ResourceRequest,
	ref WorkerClaimRef,
	epoch uint64,
) (*Reservation, error) {
	return a.reserve(ctx, workerObjectKey, executionObjectKey, request, &ref, epoch)
}

// reserve creates and debits one reservation under the Worker mutation lock.
func (a *WorldRuntimeAdmission) reserve(
	ctx context.Context,
	workerObjectKey, executionObjectKey string,
	request ResourceRequest,
	ref *WorkerClaimRef,
	epoch uint64,
) (*Reservation, error) {
	// Validate the Worker, Execution and resource request.
	if workerObjectKey == "" || executionObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "worker and execution keys")
	}
	if err := request.Validate(); err != nil {
		return nil, errors.Wrap(err, "resource request")
	}

	// Serialize the Worker reservation and capacity debit.
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	// Create or reuse the reservation in one capacity transaction.
	var out *Reservation
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Find the reservation for this Execution attempt.
		resKey := BuildReservationObjectKey(executionObjectKey)
		existing, err := LookupReservation(ctx, ws, resKey)
		if err != nil && !errors.Is(err, ErrReservationNotFound) {
			return err
		}
		if existing != nil {
			// Reuse a live matching reservation only after proving its debit.
			switch {
			case existing.State.Terminal():
				// A retry after release is a new attempt with a new Execution key.
				return ErrReservationTerminal
			case existing.LeaseExpired(a.now()):
				return ErrReservationExpired
			case existing.WorkerObjectKey != workerObjectKey || existing.Request != request:
				return ErrRequestMismatch
			}

			// Prove the debit is still present before returning idempotently.
			capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
			if err != nil {
				return err
			}
			if ref != nil {
				if err := verifyLiveClaim(capacity, *ref, a.now()); err != nil {
					return err
				}
				if capacity.OwnerEpoch != epoch {
					return ErrCapacityOwned
				}
			}
			if err := capacity.OwnerClaimActive(a.now()); err != nil {
				return err
			}
			if remainingMilliCPU(capacity)+existing.Request.MilliCPU < existing.Request.MilliCPU ||
				capacity.MilliCPUReserved < existing.Request.MilliCPU ||
				capacity.MemoryBytesReserved < existing.Request.MemoryBytes {
				return errors.Wrapf(ErrCapacityExhausted, "debit missing for %s", resKey)
			}
			out = existing
			return nil
		}

		// Require an active Worker claim with enough backend capacity.
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if err != nil {
			return err
		}
		if ref != nil {
			if err := verifyLiveClaim(capacity, *ref, a.now()); err != nil {
				return err
			}
			if capacity.OwnerEpoch != epoch {
				return ErrCapacityOwned
			}
		}
		if err := capacity.OwnerClaimActive(a.now()); err != nil {
			return err
		}
		if !capacity.SupportsBackend(request.Backend) {
			return errors.Wrapf(ErrBackendUnsupported, "backend %q", request.Backend)
		}
		if remainingMilliCPU(capacity) < request.MilliCPU || remainingMemoryBytes(capacity) < request.MemoryBytes {
			return errors.Wrapf(ErrCapacityExhausted, "worker %s", workerObjectKey)
		}

		// Create the leased reservation and debit the Worker atomically.
		now := a.now().UTC()
		res := &Reservation{
			WorkerObjectKey:    workerObjectKey,
			ExecutionObjectKey: executionObjectKey,
			Request:            request,
			Generation:         1,
			LeaseExpiresAt:     timestamp.New(now.Add(a.lease)),
			State:              ReservationStateReserved,
		}
		if err := res.Validate(); err != nil {
			return err
		}
		if err := persistNewReservation(ctx, ws, res); err != nil {
			return err
		}
		if err := debitCapacity(ctx, ws, workerObjectKey, request); err != nil {
			return err
		}
		out = res
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Activate claims the reserved capacity for one backend runtime before launch.
func (a *WorldRuntimeAdmission) Activate(
	ctx context.Context,
	reservationObjectKey string,
	rt BackendRuntimeIdentity,
) (*Reservation, error) {
	return a.activate(ctx, reservationObjectKey, rt, nil, 0)
}

// ActivateForClaim fences runtime activation against the current Worker claim.
func (a *WorldRuntimeAdmission) ActivateForClaim(
	ctx context.Context,
	reservationObjectKey string,
	rt BackendRuntimeIdentity,
	ref WorkerClaimRef,
	epoch uint64,
) (*Reservation, error) {
	return a.activate(ctx, reservationObjectKey, rt, &ref, epoch)
}

// activate records runtime custody after its claim fence passes.
func (a *WorldRuntimeAdmission) activate(
	ctx context.Context,
	reservationObjectKey string,
	rt BackendRuntimeIdentity,
	ref *WorkerClaimRef,
	epoch uint64,
) (*Reservation, error) {
	if rt.IsZero() {
		return nil, errors.New("runtime identity must be set")
	}
	return a.transitionReservationForClaim(ctx, reservationObjectKey, ref, epoch, func(res *Reservation) error {
		// Require reserved capacity before recording runtime custody.
		if res.State != ReservationStateReserved {
			return errors.Errorf("cannot activate from state %d", res.State)
		}

		// Activate the reservation and extend its custody lease.
		res.State = ReservationStateActive
		res.Runtime = rt
		renewLease(res, a.now(), a.lease)
		return nil
	})
}

// MarkUncertain records that runtime custody is unreachable while capacity
// stays debited, whether before launch or after activation. Idempotent while
// uncertain; rejected once released or pending stop.
func (a *WorldRuntimeAdmission) MarkUncertain(ctx context.Context, reservationObjectKey string) (*Reservation, error) {
	return a.transitionReservation(ctx, reservationObjectKey, func(res *Reservation) error {
		switch res.State {
		case ReservationStateUncertain:
			return nil
		case ReservationStateReserved, ReservationStateActive:
			res.State = ReservationStateUncertain
			return nil
		default:
			return errors.Errorf("cannot mark uncertain from state %d", res.State)
		}
	})
}

// ResumeFromUncertain re-fences custody when the same fenced runtime
// reconnects. The generation increments so a late return from the previous
// runtime instance cannot stop or release the re-fenced reservation.
func (a *WorldRuntimeAdmission) ResumeFromUncertain(
	ctx context.Context,
	reservationObjectKey string,
	rt BackendRuntimeIdentity,
) (*Reservation, error) {
	if rt.IsZero() {
		return nil, errors.New("runtime identity must be set")
	}
	return a.transitionReservation(ctx, reservationObjectKey, func(res *Reservation) error {
		// Require uncertain custody before resuming the reservation.
		if res.State != ReservationStateUncertain {
			return errors.Errorf("cannot resume from state %d", res.State)
		}

		// Fence the resumed runtime and renew its active lease.
		res.Generation++
		res.Runtime = rt
		res.State = ReservationStateActive
		renewLease(res, a.now(), a.lease)
		return nil
	})
}

// RenewLease extends the lease of a live reservation from the owning
// instance. The claim reference is verified against the worker record's
// durable live claim inside the transition transaction: a deposed instance
// renewing leases in a loop must not starve the new owner's expiry sweep.
func (a *WorldRuntimeAdmission) RenewLease(ctx context.Context, ref WorkerClaimRef, reservationObjectKey string) (*Reservation, error) {
	// Resolve and lock the Worker holding this reservation.
	res, err := a.LookupReservation(ctx, reservationObjectKey)
	if err != nil {
		return nil, err
	}
	unlock := a.lockWorker(res.WorkerObjectKey)
	defer unlock()

	// Renew the reservation under the current Worker claim.
	var out *Reservation
	err = a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Verify the Worker claim before changing its reservation lease.
		capacity, err := LookupWorkerCapacity(ctx, ws, res.WorkerObjectKey)
		if err != nil {
			return err
		}
		if err := verifyLiveClaim(capacity, ref, a.now()); err != nil {
			return err
		}

		// Load the current live reservation before renewal.
		current, err := LookupReservation(ctx, ws, reservationObjectKey)
		if err != nil {
			return err
		}
		if !current.State.Live() {
			return ErrReservationTerminal
		}

		// Validate and persist the renewed reservation lease.
		renewLease(current, a.now(), a.lease)
		if err := current.Validate(); err != nil {
			return err
		}
		if err := persistReservation(ctx, ws, reservationObjectKey, current); err != nil {
			return err
		}
		out = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RenewWorkerReservations extends every live reservation held by the Worker.
// The durable reservation records supply the renewal set after controller
// exits, policy transitions, and Worker restarts.
func (a *WorldRuntimeAdmission) RenewWorkerReservations(ctx context.Context, workerObjectKey string, ref WorkerClaimRef) error {
	keys, err := a.listReservationKeys(ctx)
	if err != nil {
		return err
	}
	for _, key := range keys {
		res, err := a.LookupReservation(ctx, key)
		if err != nil {
			return err
		}
		if res.WorkerObjectKey != workerObjectKey || !res.State.Live() {
			continue
		}
		if _, err := a.RenewLease(ctx, ref, key); err != nil && !errors.Is(err, ErrReservationTerminal) {
			return err
		}
	}
	return nil
}

// LookupReservation implements RuntimeAdmission.
func (a *WorldRuntimeAdmission) LookupReservation(ctx context.Context, reservationObjectKey string) (*Reservation, error) {
	var out *Reservation
	err := a.withTx(ctx, false, func(ctx context.Context, ws world.WorldState) error {
		res, err := LookupReservation(ctx, ws, reservationObjectKey)
		out = res
		return err
	})
	return out, err
}

// StopAndRelease implements RuntimeAdmission.
//
// The transition into the durable pending-stop state applies under the Worker
// lock in its own transaction. The runtime stop runs without the lock held.
// The finalizing transaction persists the confirmed receipt and credits the
// capacity atomically. A stale generation is rejected without touching the
// current runtime or capacity: that call belongs to a replaced runtime
// instance returning late. A crash between transition and finalize leaves the
// reservation durably in the pending-stop state; ReconcilePendingStops
// finishes it with the same idempotent stopper.
func (a *WorldRuntimeAdmission) StopAndRelease(
	ctx context.Context,
	ref WorkerClaimRef,
	ownerEpoch uint64,
	reservationObjectKey string,
	generation uint64,
) (*CleanupReceipt, error) {
	// Enter the durable pending-stop state or return the completed receipt.
	res, receipt, err := a.beginStopLocked(ctx, ref, ownerEpoch, reservationObjectKey, generation)
	if err != nil {
		return nil, err
	}
	if receipt != nil {
		return receipt, nil
	}

	// Stop outside the Worker lock; the stop may block on backend mechanics.
	runtimeStopped := true
	if !res.Runtime.IsZero() {
		stopped, err := a.stopRuntimeChecked(ctx, res.Runtime)
		if err != nil {
			return nil, err
		}
		runtimeStopped = stopped
	}
	return a.finalizeStop(ctx, ref, ownerEpoch, res.ObjectKey(), res.WorkerObjectKey, res.ExecutionObjectKey, res.Runtime.ID, res.Generation, runtimeStopped, CleanupReasonStop, false)
}

// ExpireLeases fences every live reservation whose lease expired at or
// before now. Each expiry is one transaction that bumps the generation,
// enters the durable pending-stop state while retaining the debit, and
// persists the explicitly partial receipt: RuntimeStopped=false because the
// runtime outcome is unknown, CapacityReleased=false because the debit is
// held until the stop confirms. After committing, the sweep attempts the stop
// outside the Worker lock; confirmation finalizes the truthful terminal
// receipt and credits the debit exactly once. Failure leaves the work to
// ReconcilePendingStops. Post-expiry calls fenced against the old generation
// are stale and cannot receive the terminal receipt.
func (a *WorldRuntimeAdmission) ExpireLeases(ctx context.Context, ref WorkerClaimRef, now time.Time) ([]*CleanupReceipt, error) {
	// List persisted reservations for the lease-expiry sweep.
	keys, err := a.listReservationKeys(ctx)
	if err != nil {
		return nil, err
	}
	var receipts []*CleanupReceipt

	// Expire each eligible reservation and collect its cleanup receipt.
	for _, key := range keys {
		// Resolve the Worker holding this reservation.
		workerKey, err := a.resolveWorkerForReservation(ctx, key)
		if errors.Is(err, ErrReservationNotFound) {
			continue
		}
		if err != nil {
			return receipts, err
		}

		// Entry check: only records carrying this live claim are swept.
		// Missing, legacy ownerless, foreign-held, and expired-claim records
		// are skipped without invoking the stopper.
		live, err := a.claimLiveForWorker(ctx, workerKey, ref)
		if err != nil {
			return receipts, err
		}
		if !live {
			continue
		}

		// Fence the expired reservation under its Worker mutation lock.
		unlock := a.lockWorker(workerKey)
		receipt, err := a.expireOne(ctx, ref, key, now)
		unlock()
		if err != nil {
			return receipts, err
		}
		if receipt == nil {
			continue
		}
		receipts = append(receipts, receipt)

		// Best-effort immediate stop outside the Worker lock;
		// ReconcilePendingStops retries later. When the stop confirms here,
		// report the completed truthful receipt instead of the partial one.
		done, err := a.reconcilePendingStop(ctx, ref, receipt.ReservationObjectKey)
		if err == nil && done != nil {
			receipts[len(receipts)-1] = done
		}
	}
	return receipts, nil
}

// claimLiveForWorker reports whether the Worker's capacity record carries the
// caller's live claim. It is the entry check for sweeps; the in-transaction
// recheck inside each mutation remains authoritative. Missing records are
// simply not live; other read failures propagate to the caller.
func (a *WorldRuntimeAdmission) claimLiveForWorker(ctx context.Context, workerObjectKey string, ref WorkerClaimRef) (bool, error) {
	var live bool
	err := a.withTx(ctx, false, func(ctx context.Context, ws world.WorldState) error {
		// Read the Worker claim and check whether the sweeping instance still holds it.
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if errors.Is(err, ErrWorkerNotObserved) {
			return nil
		}
		if err != nil {
			return err
		}
		live = verifyLiveClaim(capacity, ref, a.now()) == nil
		return nil
	})
	return live, err
}

// ReconcilePendingStops confirms stops for every pending-stop reservation by
// running the idempotent stopper and finalizing the receipt. It completes the
// work of a crashed StopAndRelease or an unreachable expired runtime.
func (a *WorldRuntimeAdmission) ReconcilePendingStops(ctx context.Context, ref WorkerClaimRef) ([]*CleanupReceipt, error) {
	// List persisted reservations for pending-stop reconciliation.
	keys, err := a.listReservationKeys(ctx)
	if err != nil {
		return nil, err
	}
	var receipts []*CleanupReceipt

	// Collect cleanup receipts from the current claim's pending stops.
	for _, key := range keys {
		// Resolve the Worker holding this reservation.
		workerKey, err := a.resolveWorkerForReservation(ctx, key)
		if errors.Is(err, ErrReservationNotFound) {
			continue
		}
		if err != nil {
			return receipts, err
		}

		// Skip Workers whose durable claim is no longer held by this instance.
		live, err := a.claimLiveForWorker(ctx, workerKey, ref)
		if err != nil {
			return receipts, err
		}
		if !live {
			continue
		}

		// Reconcile the pending runtime stop and retain any completed receipt.
		receipt, err := a.reconcilePendingStop(ctx, ref, key)
		if err != nil {
			return receipts, err
		}
		if receipt != nil {
			receipts = append(receipts, receipt)
		}
	}
	return receipts, nil
}

// expireOne expires one expired live reservation in one transaction.
// Caller holds the Worker lock. Returns nil when the reservation is not
// expirable.
func (a *WorldRuntimeAdmission) expireOne(ctx context.Context, ref WorkerClaimRef, objKey string, now time.Time) (*CleanupReceipt, error) {
	var out *CleanupReceipt
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Load an expired live reservation and verify its Worker claim.
		res, err := LookupReservation(ctx, ws, objKey)
		if err != nil {
			return err
		}
		if !res.State.Live() || !res.LeaseExpired(now) {
			return nil
		}
		capacity, err := LookupWorkerCapacity(ctx, ws, res.WorkerObjectKey)
		if err != nil {
			return err
		}
		if err := verifyLiveClaim(capacity, ref, a.now()); err != nil {
			return nil
		}

		// Fence custody: every old-generation call becomes stale. The debit
		// stays held until the stop confirms.
		fenced := *res
		fenced.Generation++
		fenced.State = ReservationStatePendingStop
		receipt := &CleanupReceipt{
			ReservationObjectKey: objKey,
			ExecutionObjectKey:   res.ExecutionObjectKey,
			RuntimeIdentity:      res.Runtime.ID,
			Generation:           fenced.Generation,
			RuntimeStopped:       false,
			CapacityReleased:     false,
			Reason:               CleanupReasonExpired,
		}
		if err := receipt.Validate(); err != nil {
			return err
		}

		// Persist the validated partial receipt with capacity still debited.
		fenced.Cleanup = receipt
		fenced.LeaseExpiresAt = timestamp.New(now)
		if err := fenced.Validate(); err != nil {
			return err
		}
		if err := persistReservation(ctx, ws, objKey, &fenced); err != nil {
			return err
		}
		out = receipt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// beginStopLocked validates the generation and moves a live reservation into
// the pending-stop state without touching capacity. Caller does not hold the
// lock; the function takes it. Returns the fenced reservation with its Worker
// key, or a non-nil receipt when the reservation already released for this
// generation.
func (a *WorldRuntimeAdmission) beginStopLocked(
	ctx context.Context,
	ref WorkerClaimRef,
	ownerEpoch uint64,
	reservationObjectKey string,
	generation uint64,
) (*Reservation, *CleanupReceipt, error) {
	// Resolve and lock the Worker holding the stopped reservation.
	res, err := a.LookupReservation(ctx, reservationObjectKey)
	if err != nil {
		return nil, nil, err
	}
	unlock := a.lockWorker(res.WorkerObjectKey)
	defer unlock()

	// Fence the reservation and persist its pending-stop transition.
	var out *Reservation
	err = a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Load the current reservation and accept an already completed generation.
		current, err := LookupReservation(ctx, ws, reservationObjectKey)
		if err != nil {
			return err
		}
		if current.State.Terminal() {
			if current.Generation != generation {
				return ErrStaleGeneration
			}
			out = current
			return nil
		}

		// Require the current Worker claim before changing runtime custody.
		if _, err := loadOwnedCapacity(ctx, ws, res.WorkerObjectKey, ref, ownerEpoch, a.now()); err != nil {
			return err
		}

		// Fence the generation and enter or resume the pending-stop state.
		switch {
		case current.Generation != generation:
			return ErrStaleGeneration
		case current.State == ReservationStatePendingStop:
			// Crash recovery or a concurrent return for the same fenced
			// generation: continue the pending stop.
			out = current
			return nil
		case current.State == ReservationStateReserved || current.State == ReservationStateActive || current.State == ReservationStateUncertain:
			fenced := *current
			fenced.State = ReservationStatePendingStop
			if err := fenced.Validate(); err != nil {
				return err
			}
			if err := persistReservation(ctx, ws, reservationObjectKey, &fenced); err != nil {
				return err
			}
			out = &fenced
			return nil
		default:
			return errors.Errorf("cannot stop from state %d", current.State)
		}
	})
	if err != nil {
		return nil, nil, err
	}
	if out.State.Terminal() {
		return nil, out.Cleanup, nil
	}
	return out, nil, nil
}

// finalizeStop persists the stop facts in one transaction. The persisted
// fence generation must match the receipt generation. When the stop did not
// confirm yet, the explicitly partial receipt (RuntimeStopped=false and
// CapacityReleased=false) stays durable under the pending-stop state with the
// debit held; reconciliation finishes it. When the stop confirms, the same
// transaction records the terminal receipt and credits the debit exactly
// once: only the released state releases capacity.
func (a *WorldRuntimeAdmission) finalizeStop(
	ctx context.Context,
	ref WorkerClaimRef,
	ownerEpoch uint64,
	objKey, workerObjectKey, executionObjectKey, runtimeIdentity string,
	generation uint64,
	runtimeStopped bool,
	reason string,
	benignOnFence bool,
) (*CleanupReceipt, error) {
	// Serialize finalization of the Worker reservation.
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	// Record the stop receipt and capacity credit atomically.
	var out *CleanupReceipt
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Recheck the Worker claim after stopping the backend runtime.
		if _, err := loadOwnedCapacity(ctx, ws, workerObjectKey, ref, ownerEpoch, a.now()); err != nil {
			// Claim turnover between stop and finalize: for sweep/reconcile
			// this is a benign skip. The partial receipt and debit stay
			// durable so the new owner's reconcile finishes the release.
			if benignOnFence && isClaimFenceError(err) {
				return nil
			}
			return err
		}

		// Require the same pending reservation and Execution generation.
		res, err := LookupReservation(ctx, ws, objKey)
		if err != nil {
			return err
		}
		if res.Generation != generation {
			return ErrStaleGeneration
		}
		if res.ExecutionObjectKey != executionObjectKey {
			return ErrRequestMismatch
		}
		if res.State.Terminal() {
			out = res.Cleanup
			return nil
		}
		if res.State != ReservationStatePendingStop {
			return errors.Errorf("cannot finalize from state %d", res.State)
		}

		// Preserve the cleanup reason from the pending-stop receipt.
		prior := res.Cleanup
		if prior != nil && prior.Reason != "" {
			reason = prior.Reason
		}

		// Retain the partial cleanup receipt while the runtime stop is unconfirmed.
		if !runtimeStopped {
			// The stop did not confirm; keep the truthful partial receipt and
			// hold the debit for reconciliation.
			receipt := &CleanupReceipt{
				ReservationObjectKey: objKey,
				ExecutionObjectKey:   res.ExecutionObjectKey,
				RuntimeIdentity:      runtimeIdentity,
				Generation:           generation,
				RuntimeStopped:       false,
				CapacityReleased:     false,
				Reason:               reason,
			}
			partial := *res
			partial.Cleanup = receipt
			if err := partial.Validate(); err != nil {
				return err
			}
			if err := persistReservation(ctx, ws, objKey, &partial); err != nil {
				return err
			}
			out = receipt
			return nil
		}

		// Complete the receipt and release the reservation after a confirmed stop.
		receipt := &CleanupReceipt{
			ReservationObjectKey: objKey,
			ExecutionObjectKey:   res.ExecutionObjectKey,
			RuntimeIdentity:      runtimeIdentity,
			Generation:           generation,
			RuntimeStopped:       true,
			CapacityReleased:     true,
			Reason:               reason,
		}
		finalized := *res
		finalized.State = ReservationStateReleased
		finalized.Cleanup = receipt
		if err := finalized.Validate(); err != nil {
			return err
		}

		// Persist the release and credit the Worker capacity exactly once.
		if err := persistReservation(ctx, ws, objKey, &finalized); err != nil {
			return err
		}
		if err := creditCapacity(ctx, ws, res.WorkerObjectKey, res.Request); err != nil {
			return err
		}
		out = receipt
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// reconcilePendingStop confirms the stop of one pending-stop reservation.
// Returns the completed receipt, or nil when the key is not pending-stop.
func (a *WorldRuntimeAdmission) reconcilePendingStop(ctx context.Context, ref WorkerClaimRef, objKey string) (*CleanupReceipt, error) {
	// Load the reservation and skip records outside pending-stop.
	res, err := a.LookupReservation(ctx, objKey)
	if err != nil {
		return nil, err
	}
	if res.State != ReservationStatePendingStop {
		return nil, nil
	}

	// Entry check before any stopper invocation: a deposed or stale instance
	// must not stop runtimes it no longer owns.
	capacity, err := a.LookupWorkerCapacityAdmission(ctx, res.WorkerObjectKey)
	if err != nil {
		return nil, err
	}
	if err := verifyLiveClaim(capacity, ref, a.now()); err != nil {
		return nil, nil
	}

	// Confirm the backend runtime stop before crediting capacity.
	runtimeStopped := true
	if !res.Runtime.IsZero() {
		stopped, err := a.stopRuntimeChecked(ctx, res.Runtime)
		if err != nil {
			return nil, err
		}
		runtimeStopped = stopped
	}

	// Preserve the original cleanup reason when completing the receipt.
	priorReason := ""
	if res.Cleanup != nil {
		priorReason = res.Cleanup.Reason
	}
	if priorReason == "" {
		priorReason = CleanupReasonStop
	}

	// Finalize the receipt against the Worker claim used for the stop.
	receipt, err := a.finalizeStop(
		ctx,
		ref,
		capacity.OwnerEpoch,
		objKey,
		res.WorkerObjectKey,
		res.ExecutionObjectKey,
		res.Runtime.ID,
		res.Generation,
		runtimeStopped,
		priorReason,
		true,
	)
	if receipt == nil && err == nil {
		// Claim turnover during finalize: benign skip, debit preserved for
		// the new owner's reconcile.
		return nil, nil
	}
	return receipt, err
}

// isClaimFenceError reports whether an error came from the owner-claim fence
// rather than from storage or validation.
func isClaimFenceError(err error) bool {
	return errors.Is(err, ErrCapacityUnowned) ||
		errors.Is(err, ErrCapacityOwned) ||
		errors.Is(err, ErrCapacityOwnerExpired) ||
		errors.Is(err, ErrStaleGeneration)
}

// LookupWorkerCapacityAdmission loads one capacity record through the
// admission instance's read transaction.
func (a *WorldRuntimeAdmission) LookupWorkerCapacityAdmission(ctx context.Context, workerObjectKey string) (*WorkerCapacity, error) {
	var out *WorkerCapacity
	err := a.withTx(ctx, false, func(ctx context.Context, ws world.WorldState) error {
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		out = capacity
		return err
	})
	return out, err
}

// stopRuntimeChecked invokes the configured stopper and wraps failures.
func (a *WorldRuntimeAdmission) stopRuntimeChecked(ctx context.Context, rt BackendRuntimeIdentity) (bool, error) {
	if a.stopper == nil {
		return false, errors.New("runtime stopper not configured")
	}
	stopped, err := a.stopper.StopRuntime(ctx, rt)
	if err != nil {
		return false, errors.Wrapf(err, "stop runtime %s", rt.ID)
	}
	return stopped, nil
}

// resolveWorkerForReservation returns the owning Worker object key of a
// persisted reservation, empty when the key is unknown.
func (a *WorldRuntimeAdmission) resolveWorkerForReservation(ctx context.Context, objKey string) (string, error) {
	res, err := a.LookupReservation(ctx, objKey)
	if err != nil {
		return "", err
	}
	return res.WorkerObjectKey, nil
}

// listReservationKeys lists all persisted reservation object keys.
func (a *WorldRuntimeAdmission) listReservationKeys(ctx context.Context) ([]string, error) {
	var keys []string
	err := a.withTx(ctx, false, func(ctx context.Context, ws world.WorldState) error {
		var err error
		keys, err = listReservationKeys(ctx, ws)
		return err
	})
	return keys, err
}

// transitionReservation loads, mutates, and persists one reservation under the
// owning Worker's lock in one transaction.
func (a *WorldRuntimeAdmission) transitionReservation(
	ctx context.Context,
	reservationObjectKey string,
	cb func(*Reservation) error,
) (*Reservation, error) {
	return a.transitionReservationForClaim(ctx, reservationObjectKey, nil, 0, cb)
}

// transitionReservationForClaim applies an optional claim fence in the same
// transaction as the reservation transition.
func (a *WorldRuntimeAdmission) transitionReservationForClaim(
	ctx context.Context,
	reservationObjectKey string,
	ref *WorkerClaimRef,
	epoch uint64,
	cb func(*Reservation) error,
) (*Reservation, error) {
	// Resolve and lock the Worker holding the reservation.
	res, err := a.LookupReservation(ctx, reservationObjectKey)
	if err != nil {
		return nil, err
	}
	unlock := a.lockWorker(res.WorkerObjectKey)
	defer unlock()

	// Apply and persist the reservation transition in one transaction.
	var out *Reservation
	err = a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		// Load the current reservation and fence any supplied Worker claim.
		current, err := LookupReservation(ctx, ws, reservationObjectKey)
		if err != nil {
			return err
		}
		if ref != nil {
			if _, err := loadOwnedCapacity(ctx, ws, current.WorkerObjectKey, *ref, epoch, a.now()); err != nil {
				return err
			}
		}

		// Apply the requested reservation transition and validate its result.
		if err := cb(current); err != nil {
			return err
		}
		if err := current.Validate(); err != nil {
			return err
		}

		// Persist the changed reservation for the caller.
		if err := persistReservation(ctx, ws, reservationObjectKey, current); err != nil {
			return err
		}
		out = current
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
