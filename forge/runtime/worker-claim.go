package forge_runtime

import (
	"context"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
)

// ClaimWorkerCapacity takes the single-writer claim on one Worker's capacity
// record for the named admission owner instance. A live claim held by another
// instance fails with ErrWorkerClaimHeld; an expired claim transfers. A fresh
// claim clears a stale draining flag from the previous owner and creates the
// record when absent, so a restarted daemon claims before it observes. The
// claim lease reuses the reservation lease duration.
func (a *WorldRuntimeAdmission) ClaimWorkerCapacity(
	ctx context.Context,
	workerObjectKey, instanceID string,
) (*WorkerCapacity, error) {
	if workerObjectKey == "" || instanceID == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "worker_object_key and instance_id")
	}
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	var out *WorkerCapacity
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if err != nil && !errors.Is(err, ErrWorkerNotObserved) {
			return err
		}
		now := a.now().UTC()
		if capacity == nil {
			capacity = &WorkerCapacity{
				ObservedAt: timestamp.New(now),
			}
		} else if capacity.ClaimInstance != instanceID && capacity.ClaimLive(capacity.ClaimInstance, now) {
			return errors.Wrapf(ErrWorkerClaimHeld, "worker %s held by %s", workerObjectKey, capacity.ClaimInstance)
		} else if capacity.ClaimInstance != instanceID {
			// Takeover from an expired or absent owner starts a clean drain
			// state for the new owner.
			capacity.Draining = false
		}
		capacity.WorkerObjectKey = workerObjectKey
		capacity.ClaimInstance = instanceID
		capacity.ClaimExpiresAt = timestamp.New(now.Add(a.lease))
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

// RenewWorkerClaim extends the calling instance's claim lease while it stays
// active. An expired or transferred claim fails with ErrWorkerNotClaimed so
// the caller stops mutating a record another owner may hold.
func (a *WorldRuntimeAdmission) RenewWorkerClaim(
	ctx context.Context,
	workerObjectKey, instanceID string,
) (*WorkerCapacity, error) {
	return a.mutateClaimedCapacity(ctx, workerObjectKey, instanceID, func(capacity *WorkerCapacity, now time.Time) error {
		capacity.ClaimExpiresAt = timestamp.New(now.UTC().Add(a.lease))
		return nil
	})
}

// BeginDrainCapacity marks the claimed Worker draining. Reserve rejects new
// attempts on this Worker until CompleteDrainCapacity removes the drained
// record; existing reservations release through their normal fence.
func (a *WorldRuntimeAdmission) BeginDrainCapacity(
	ctx context.Context,
	workerObjectKey, instanceID string,
) (*WorkerCapacity, error) {
	return a.mutateClaimedCapacity(ctx, workerObjectKey, instanceID, func(capacity *WorkerCapacity, _ time.Time) error {
		capacity.Draining = true
		return nil
	})
}

// CompleteDrainCapacity removes the drained capacity record once no reserved
// debit remains. Live reservations fail with ErrDrainIncomplete: the record
// stays until every debit credits back through its release.
func (a *WorldRuntimeAdmission) CompleteDrainCapacity(
	ctx context.Context,
	workerObjectKey, instanceID string,
) error {
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	return a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if err != nil {
			return err
		}
		now := a.now()
		if !capacity.ClaimLive(instanceID, now) {
			return errors.Wrapf(ErrWorkerNotClaimed, "worker %s", workerObjectKey)
		}
		if !capacity.Draining {
			return errors.Wrapf(ErrDrainIncomplete, "worker %s is not draining", workerObjectKey)
		}
		if capacity.MilliCPUReserved != 0 || capacity.MemoryBytesReserved != 0 {
			return errors.Wrapf(ErrDrainIncomplete, "worker %s still holds debits", workerObjectKey)
		}
		found, err := ws.DeleteObject(ctx, BuildWorkerCapacityObjectKey(workerObjectKey))
		if err != nil {
			return errors.Wrapf(err, "delete drained capacity %s", workerObjectKey)
		}
		if !found {
			return errors.Wrapf(world.ErrObjectNotFound, "capacity %s", workerObjectKey)
		}
		return nil
	})
}

// ScanOwnedCapacity lists Workers whose unexpired claim names instanceID. A
// restarted daemon uses this to reclaim its records instead of guessing which
// Workers it served before.
func (a *WorldRuntimeAdmission) ScanOwnedCapacity(ctx context.Context, instanceID string) ([]string, error) {
	if instanceID == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "instance_id")
	}
	var owned []string
	err := a.withTx(ctx, false, func(ctx context.Context, ws world.WorldState) error {
		it := ws.IterateObjects(ctx, workerCapacityObjectKeyPrefix, false)
		defer it.Close()
		now := a.now()
		for it.Next() {
			if !it.Valid() {
				break
			}
			capacity, err := world.LookupObjectBody[*WorkerCapacity](ctx, ws, it.Key(), NewWorkerCapacityBlock)
			if err != nil {
				return err
			}
			if capacity.ClaimLive(instanceID, now) {
				owned = append(owned, capacity.WorkerObjectKey)
			}
		}
		return it.Err()
	})
	if err != nil {
		return nil, err
	}
	return owned, nil
}

// mutateClaimedCapacity loads one claimed capacity record under the Worker's
// lock, applies cb when the calling instance holds a live claim, and persists
// the result in one transaction.
func (a *WorldRuntimeAdmission) mutateClaimedCapacity(
	ctx context.Context,
	workerObjectKey, instanceID string,
	cb func(*WorkerCapacity, time.Time) error,
) (*WorkerCapacity, error) {
	if workerObjectKey == "" || instanceID == "" {
		return nil, errors.Wrap(world.ErrEmptyObjectKey, "worker_object_key and instance_id")
	}
	unlock := a.lockWorker(workerObjectKey)
	defer unlock()

	var out *WorkerCapacity
	err := a.withTx(ctx, true, func(ctx context.Context, ws world.WorldState) error {
		capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
		if err != nil {
			return err
		}
		now := a.now()
		if !capacity.ClaimLive(instanceID, now) {
			return errors.Wrapf(ErrWorkerNotClaimed, "worker %s", workerObjectKey)
		}
		if err := cb(capacity, now); err != nil {
			return err
		}
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
