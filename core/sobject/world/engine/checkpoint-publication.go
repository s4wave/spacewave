package sobject_world_engine

import (
	"context"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
)

// reserveCheckpointPublication pairs intent and capacity without waiting while
// owning either. A retry body may already own capacity and call state metadata;
// keeping checkpoint intent across a capacity wait would deadlock that body.
func reserveCheckpointPublication(ctx context.Context, so sobject.SharedObject, store block.PublicationReserver) (block.PublicationReservation, sobject.StateWriteReservation, error) {
	// Stores without state preparation retain their ordinary capacity claim.
	reserver := stateWriteReserver(so)
	if reserver == nil {
		claim, err := store.ReservePublication(ctx, 0)
		return claim, nil, err
	}
	for {
		// Try capacity under intent, returning intent before a capacity wait.
		state, err := reserver.ReserveStateWrite(ctx, false)
		if err != nil {
			return nil, nil, err
		}
		if state == nil {
			capacity, err := store.ReservePublication(ctx, 0)
			return capacity, nil, err
		}
		capacity, err := store.TryReservePublication(ctx, 0)
		if err != nil {
			state.Release()
			return nil, nil, err
		}
		if capacity != nil {
			return capacity, state, nil
		}
		state.Release()

		// Try intent under capacity, returning capacity before an intent wait.
		capacity, err = store.ReservePublication(ctx, 0)
		if err != nil {
			return nil, nil, err
		}
		state, err = reserver.TryReserveStateWrite(ctx, false)
		if err != nil {
			if capacity != nil {
				capacity.Release()
			}
			return nil, nil, err
		}
		if state != nil {
			return capacity, state, nil
		}
		if capacity != nil {
			capacity.Release()
		}
	}
}
