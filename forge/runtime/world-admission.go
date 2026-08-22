package forge_runtime

import (
	"context"
	"sync"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
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
// capacity record. A second daemon writing the same Worker's capacity record
// is outside this contract and must not be attempted: run one admission owner
// per Worker.
type WorldRuntimeAdmission struct {
	// eng is the world engine holding the durable state.
	eng world.Engine
	// stopper stops backend runtimes during release and reconciliation.
	stopper RuntimeStopper
	// lease is the reservation lease duration.
	lease time.Duration
	// now returns the current time; overridable in tests.
	now func() time.Time

	mtx sync.Mutex
	// workerLocks serialize capacity mutations per Worker object key.
	workerLocks map[string]*sync.Mutex
}

// NewWorldRuntimeAdmission constructs a world-backed RuntimeAdmission.
// A zero lease uses DefaultLeaseDuration.
func NewWorldRuntimeAdmission(eng world.Engine, stopper RuntimeStopper, lease time.Duration) *WorldRuntimeAdmission {
	if lease == 0 {
		lease = DefaultLeaseDuration
	}
	return &WorldRuntimeAdmission{
		eng:         eng,
		stopper:     stopper,
		lease:       lease,
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
	a.mtx.Lock()
	lk, ok := a.workerLocks[workerObjectKey]
	if !ok {
		lk = &sync.Mutex{}
		a.workerLocks[workerObjectKey] = lk
	}
	a.mtx.Unlock()
	lk.Lock()
	return lk.Unlock
}

// withTx runs cb inside one world transaction of the given mode.
func (a *WorldRuntimeAdmission) withTx(
	ctx context.Context,
	write bool,
	cb func(ctx context.Context, ws world.WorldState) error,
) error {
	return world.ExecTransaction(ctx, a.eng, write, func(ctx context.Context, wtx world.WorldState) error {
		return cb(ctx, wtx)
	})
}

// ObserveWorker upserts the Worker's observed capacity totals and backends.
// Reserved debits are preserved; every observation bumps the record generation.
func (a *WorldRuntimeAdmission) ObserveWorker(
	ctx context.Context,
	workerObjectKey string,
	milliCPUTotal, memoryBytesTotal uint64,
	backends []string,
) (*WorkerCapacity, error) {
	if workerObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "worker_object_key")
	}
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	var out *WorkerCapacity
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if err != nil && !errors.Is(err, ErrWorkerNotObserved) {
			return err
		}
		if capacity == nil {
			capacity = &WorkerCapacity{}
		}
		capacity.MilliCPUTotal = milliCPUTotal
		capacity.MemoryBytesTotal = memoryBytesTotal
		capacity.Backends = append([]string(nil), backends...)
		capacity.ObservedAt = timestamp.Now()
		capacity.Generation++
		if err := capacity.Validate(); err != nil {
			return err
		}
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

// Reserve implements RuntimeAdmission. Creation and the capacity debit apply
// in one transaction; the idempotent return proves the debit by re-reading the
// capacity record before returning.
func (a *WorldRuntimeAdmission) Reserve(
	ctx context.Context,
	workerObjectKey, executionObjectKey string,
	request ResourceRequest,
) (*Reservation, error) {
	if workerObjectKey == "" || executionObjectKey == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "worker and execution keys")
	}
	if err := request.Validate(); err != nil {
		return nil, errors.Wrap(err, "resource request")
	}

	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	var out *Reservation
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		resKey := BuildReservationObjectKey(executionObjectKey)
		existing, err := LookupReservation(ctx, ws, resKey)
		if err != nil && !errors.Is(err, ErrReservationNotFound) {
			return err
		}
		if existing != nil {
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
			if remainingMilliCPU(capacity)+existing.Request.MilliCPU < existing.Request.MilliCPU ||
				capacity.MilliCPUReserved < existing.Request.MilliCPU ||
				capacity.MemoryBytesReserved < existing.Request.MemoryBytes {
				return errors.Wrapf(ErrCapacityExhausted, "debit missing for %s", resKey)
			}
			out = existing
			return nil
		}

		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if err != nil {
			return err
		}
		if !capacity.SupportsBackend(request.Backend) {
			return errors.Wrapf(ErrBackendUnsupported, "backend %q", request.Backend)
		}
		if remainingMilliCPU(capacity) < request.MilliCPU || remainingMemoryBytes(capacity) < request.MemoryBytes {
			return errors.Wrapf(ErrCapacityExhausted, "worker %s", workerObjectKey)
		}

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
	if rt.IsZero() {
		return nil, errors.New("runtime identity must be set")
	}
	return a.transitionReservation(ctx, reservationObjectKey, func(res *Reservation) error {
		if res.State != ReservationStateReserved {
			return errors.Errorf("cannot activate from state %d", res.State)
		}
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
		if res.State != ReservationStateUncertain {
			return errors.Errorf("cannot resume from state %d", res.State)
		}
		res.Generation++
		res.Runtime = rt
		res.State = ReservationStateActive
		renewLease(res, a.now(), a.lease)
		return nil
	})
}

// RenewLease extends the lease of a live reservation from the observing owner.
func (a *WorldRuntimeAdmission) RenewLease(ctx context.Context, reservationObjectKey string) (*Reservation, error) {
	return a.transitionReservation(ctx, reservationObjectKey, func(res *Reservation) error {
		if !res.State.Live() {
			return ErrReservationTerminal
		}
		renewLease(res, a.now(), a.lease)
		return nil
	})
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
	reservationObjectKey string,
	generation uint64,
) (*CleanupReceipt, error) {
	res, receipt, err := a.beginStopLocked(ctx, reservationObjectKey, generation)
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
	return a.finalizeStop(ctx, res.ObjectKey(), res.WorkerObjectKey, res.ExecutionObjectKey, res.Runtime.ID, res.Generation, runtimeStopped, CleanupReasonStop)
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
func (a *WorldRuntimeAdmission) ExpireLeases(ctx context.Context, now time.Time) ([]*CleanupReceipt, error) {
	keys, err := a.listReservationKeys(ctx)
	if err != nil {
		return nil, err
	}
	var receipts []*CleanupReceipt
	for _, key := range keys {
		workerKey, err := a.resolveWorkerForReservation(ctx, key)
		if err != nil {
			return receipts, err
		}
		unlock := a.lockWorker(workerKey)
		receipt, err := a.expireOne(ctx, key, now)
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
		done, err := a.reconcilePendingStop(ctx, receipt.ReservationObjectKey)
		if err == nil && done != nil {
			receipts[len(receipts)-1] = done
		}
	}
	return receipts, nil
}

// ReconcilePendingStops confirms stops for every pending-stop reservation by
// running the idempotent stopper and finalizing the receipt. It completes the
// work of a crashed StopAndRelease or an unreachable expired runtime.
func (a *WorldRuntimeAdmission) ReconcilePendingStops(ctx context.Context) ([]*CleanupReceipt, error) {
	keys, err := a.listReservationKeys(ctx)
	if err != nil {
		return nil, err
	}
	var receipts []*CleanupReceipt
	for _, key := range keys {
		receipt, err := a.reconcilePendingStop(ctx, key)
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
func (a *WorldRuntimeAdmission) expireOne(ctx context.Context, objKey string, now time.Time) (*CleanupReceipt, error) {
	var out *CleanupReceipt
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		res, err := LookupReservation(ctx, ws, objKey)
		if err != nil {
			return err
		}
		if !res.State.Live() || !res.LeaseExpired(now) {
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
	reservationObjectKey string,
	generation uint64,
) (*Reservation, *CleanupReceipt, error) {
	res, err := a.LookupReservation(ctx, reservationObjectKey)
	if err != nil {
		return nil, nil, err
	}
	unlock := a.lockWorker(res.WorkerObjectKey)
	defer unlock()

	var out *Reservation
	err = a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		current, err := LookupReservation(ctx, ws, reservationObjectKey)
		if err != nil {
			return err
		}
		switch {
		case current.State.Terminal():
			if current.Generation != generation {
				return ErrStaleGeneration
			}
			out = current
			return nil
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
	objKey, workerObjectKey, executionObjectKey, runtimeIdentity string,
	generation uint64,
	runtimeStopped bool,
	reason string,
) (*CleanupReceipt, error) {
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	var out *CleanupReceipt
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
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

		prior := res.Cleanup
		if prior != nil && prior.Reason != "" {
			reason = prior.Reason
		}
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
func (a *WorldRuntimeAdmission) reconcilePendingStop(ctx context.Context, objKey string) (*CleanupReceipt, error) {
	res, err := a.LookupReservation(ctx, objKey)
	if err != nil {
		return nil, err
	}
	if res.State != ReservationStatePendingStop {
		return nil, nil
	}
	runtimeStopped := true
	if !res.Runtime.IsZero() {
		stopped, err := a.stopRuntimeChecked(ctx, res.Runtime)
		if err != nil {
			return nil, err
		}
		runtimeStopped = stopped
	}
	priorReason := ""
	if res.Cleanup != nil {
		priorReason = res.Cleanup.Reason
	}
	if priorReason == "" {
		priorReason = CleanupReasonStop
	}
	return a.finalizeStop(
		ctx,
		objKey,
		res.WorkerObjectKey,
		res.ExecutionObjectKey,
		res.Runtime.ID,
		res.Generation,
		runtimeStopped,
		priorReason,
	)
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
	res, err := a.LookupReservation(ctx, reservationObjectKey)
	if err != nil {
		return nil, err
	}
	unlock := a.lockWorker(res.WorkerObjectKey)
	defer unlock()

	var out *Reservation
	err = a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		current, err := LookupReservation(ctx, ws, reservationObjectKey)
		if err != nil {
			return err
		}
		if err := cb(current); err != nil {
			return err
		}
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
