package forge_runtime

import (
	"context"
	"slices"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// persistReservation writes one Reservation body into the world object.
func persistReservation(ctx context.Context, ws world.WorldState, objKey string, res *Reservation) error {
	if _, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(res, true)
		return nil
	}); err != nil {
		return errors.Wrapf(err, "persist reservation %s", objKey)
	}
	return nil
}

// persistNewReservation writes a Reservation that must not exist yet.
func persistNewReservation(ctx context.Context, ws world.WorldState, res *Reservation) error {
	// Require a new reservation key before writing its body.
	objKey := res.ObjectKey()
	exists, err := ws.HasObject(ctx, objKey)
	if err != nil {
		return err
	}
	if exists {
		return errors.Wrapf(world.ErrObjectExists, "reservation %s", objKey)
	}
	return persistReservation(ctx, ws, objKey, res)
}

// listReservationKeys lists all persisted reservation object keys.
func listReservationKeys(ctx context.Context, ws world.WorldState) ([]string, error) {
	// Open an iterator over persisted reservation keys.
	var keys []string
	it := ws.IterateObjects(ctx, reservationObjectKeyPrefix, false)
	defer it.Close()

	// Collect valid reservation keys from the World index.
	for it.Next() {
		if !it.Valid() {
			break
		}
		keys = append(keys, it.Key())
	}
	return keys, it.Err()
}

// listWorkerCapacityKeys lists all persisted worker capacity object keys.
func listWorkerCapacityKeys(ctx context.Context, ws world.WorldState) ([]string, error) {
	// Open an iterator over persisted Worker capacity keys.
	var keys []string
	it := ws.IterateObjects(ctx, workerCapacityObjectKeyPrefix, false)
	defer it.Close()

	// Collect valid Worker capacity keys from the World index.
	for it.Next() {
		if !it.Valid() {
			break
		}
		keys = append(keys, it.Key())
	}
	return keys, it.Err()
}

// deleteWorkerCapacity deletes the capacity record of one Worker.
func deleteWorkerCapacity(ctx context.Context, ws world.WorldState, workerObjectKey string) error {
	_, err := ws.DeleteObject(ctx, BuildWorkerCapacityObjectKey(workerObjectKey))
	return err
}

// persistWorkerCapacity writes the capacity record of one Worker.
func persistWorkerCapacity(ctx context.Context, ws world.WorldState, workerObjectKey string, capacity *WorkerCapacity) error {
	if _, _, err := world.AccessWorldObject(
		ctx,
		ws,
		BuildWorkerCapacityObjectKey(workerObjectKey),
		true,
		func(bcs *block.Cursor) error {
			bcs.SetBlock(capacity, true)
			return nil
		},
	); err != nil {
		return errors.Wrapf(err, "persist worker capacity %s", workerObjectKey)
	}
	return nil
}

// remainingMilliCPU returns the unreserved milli-cores of one capacity record.
func remainingMilliCPU(capacity *WorkerCapacity) uint64 {
	if capacity.MilliCpuReserved > capacity.MilliCpuTotal {
		return 0
	}
	return capacity.MilliCpuTotal - capacity.MilliCpuReserved
}

// remainingMemoryBytes returns the unreserved memory bytes of one capacity record.
func remainingMemoryBytes(capacity *WorkerCapacity) uint64 {
	if capacity.MemoryBytesReserved > capacity.MemoryBytesTotal {
		return 0
	}
	return capacity.MemoryBytesTotal - capacity.MemoryBytesReserved
}

// debitCapacity adds one reservation's request and cards to the Worker's
// reserved totals in the same transaction that creates the reservation.
func debitCapacity(ctx context.Context, ws world.WorldState, res *Reservation) error {
	// Load the Worker capacity before applying its reservation debit.
	workerObjectKey := res.GetWorkerObjectKey()
	capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
	if err != nil {
		return err
	}

	// Increase and validate the Worker reserved capacity totals; validation
	// refuses a card that is not offered or is already reserved.
	capacity.MilliCpuReserved += res.GetRequest().GetMilliCpu()
	capacity.MemoryBytesReserved += res.GetRequest().GetMemoryBytes()
	capacity.ReservedCardIds = append(capacity.ReservedCardIds, res.GetCardIds()...)
	capacity.Generation++
	if err := capacity.Validate(); err != nil {
		return errors.Wrapf(err, "debit worker %s", workerObjectKey)
	}
	return persistWorkerCapacity(ctx, ws, workerObjectKey, capacity)
}

// creditCapacity removes one released reservation's request and cards from
// the Worker's reserved totals exactly once per release. This is the only path
// that reactivates a draining record whose declared backends remain: once the
// remaining debits fit the declared totals, the claim returns to ACTIVE.
func creditCapacity(ctx context.Context, ws world.WorldState, res *Reservation) error {
	// Load the Worker capacity and prevent a reservation credit underflow.
	workerObjectKey := res.GetWorkerObjectKey()
	capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
	if err != nil {
		return err
	}
	switch {
	case !capacity.HoldsDebit(res):
		return errors.Errorf("credit underflow for worker %s", workerObjectKey)
	case capacity.MilliCpuReserved == 0 && capacity.MemoryBytesReserved == 0 && len(capacity.ReservedCardIds) == 0:
		return nil
	}

	// Credit the reservation and reactivate capacity when its totals fit.
	capacity.MilliCpuReserved -= res.GetRequest().GetMilliCpu()
	capacity.MemoryBytesReserved -= res.GetRequest().GetMemoryBytes()
	released := idSet(res.GetCardIds())
	capacity.ReservedCardIds = slices.DeleteFunc(capacity.ReservedCardIds, func(id string) bool {
		_, ok := released[id]
		return ok
	})
	if capacity.OwnerState == CapacityOwnerState_CAPACITY_OWNER_STATE_DRAINING && len(capacity.Backends) > 0 &&
		capacity.fitsTotals() {
		capacity.OwnerState = CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE
	}

	// Validate the credited Worker capacity generation.
	capacity.Generation++
	if err := capacity.Validate(); err != nil {
		return errors.Wrapf(err, "credit worker %s", workerObjectKey)
	}
	return persistWorkerCapacity(ctx, ws, workerObjectKey, capacity)
}

// renewLease extends the lease of one live reservation.
func renewLease(res *Reservation, now time.Time, lease time.Duration) {
	res.LeaseExpiresAt = timestamp.New(now.UTC().Add(lease))
}
