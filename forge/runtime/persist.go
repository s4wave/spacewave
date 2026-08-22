package forge_runtime

import (
	"context"
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
	var keys []string
	it := ws.IterateObjects(ctx, reservationObjectKeyPrefix, false)
	defer it.Close()
	for it.Next() {
		if !it.Valid() {
			break
		}
		keys = append(keys, it.Key())
	}
	return keys, it.Err()
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
	if capacity.MilliCPUReserved > capacity.MilliCPUTotal {
		return 0
	}
	return capacity.MilliCPUTotal - capacity.MilliCPUReserved
}

// remainingMemoryBytes returns the unreserved memory bytes of one capacity record.
func remainingMemoryBytes(capacity *WorkerCapacity) uint64 {
	if capacity.MemoryBytesReserved > capacity.MemoryBytesTotal {
		return 0
	}
	return capacity.MemoryBytesTotal - capacity.MemoryBytesReserved
}

// debitCapacity adds one reservation's request to the Worker's reserved totals
// in the same transaction that creates the reservation.
func debitCapacity(ctx context.Context, ws world.WorldState, workerObjectKey string, request ResourceRequest) error {
	capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
	if err != nil {
		return err
	}
	capacity.MilliCPUReserved += request.MilliCPU
	capacity.MemoryBytesReserved += request.MemoryBytes
	capacity.Generation++
	if err := capacity.Validate(); err != nil {
		return errors.Wrapf(err, "debit worker %s", workerObjectKey)
	}
	return persistWorkerCapacity(ctx, ws, workerObjectKey, capacity)
}

// creditCapacity removes one released reservation's request from the Worker's
// reserved totals exactly once per release.
func creditCapacity(ctx context.Context, ws world.WorldState, workerObjectKey string, request ResourceRequest) error {
	capacity, err := LookupWorkerCapacity(ctx, ws, workerObjectKey)
	if err != nil {
		return err
	}
	switch {
	case capacity.MilliCPUReserved < request.MilliCPU || capacity.MemoryBytesReserved < request.MemoryBytes:
		return errors.Wrapf(ErrCapacityExhausted, "credit underflow for worker %s", workerObjectKey)
	case capacity.MilliCPUReserved == 0 && capacity.MemoryBytesReserved == 0:
		return nil
	}
	capacity.MilliCPUReserved -= request.MilliCPU
	capacity.MemoryBytesReserved -= request.MemoryBytes
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
