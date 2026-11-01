package sobject_world_engine

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// replayPublicationRequest retains only the latest replay save's capacity need.
// The engine's installed World and replay retain its data until it is rebuilt.
type replayPublicationRequest struct {
	// bytes is the last prepared save's block payload size.
	bytes int64
}

// replayReservationKey carries a worker's claim only to replay publication.
type replayReservationKey struct {
	// engine scopes the claim to the engine that reserved its replay save.
	engine *soEngine
}

// prepareReplayPublication never waits under the engine writer. A full queue
// defers the save; the worker reserves outside the writer and rebuilds the
// current replay under it before using that claim.
func (e *soEngine) prepareReplayPublication(ctx context.Context, entries []*block.PutBatchEntry) (block.PublicationReservation, bool, error) {
	// Placements without reservations retain their existing persistence path.
	store, ok := e.so.GetBlockStore().(block.PublicationReserver)
	if !ok || !store.SupportsPublicationReservations() {
		return nil, true, nil
	}
	var size int64
	for _, entry := range entries {
		size += int64(len(entry.Data))
	}

	// A worker's older estimate may be too small after intervening replay.
	claim, _ := ctx.Value(replayReservationKey{e}).(block.PublicationReservation)
	if claim != nil && claim.Bytes() < size {
		claim.Release()
		claim = nil
	}
	var err error
	if claim == nil {
		claim, err = store.TryReservePublication(ctx, size)
	}
	if err != nil {
		return nil, false, err
	}
	if claim != nil {
		return claim, true, nil
	}

	// Coalesce full-queue saves without retaining a second copy of the data.
	e.replayNext.Store(&replayPublicationRequest{bytes: size})
	select {
	case e.replayWake <- struct{}{}:
	default:
	}
	return nil, false, nil
}

// executeReplayPublications waits for capacity outside the engine writer, then
// rebuilds the latest replay save. Retirement cancels both admission and locking.
func (e *soEngine) executeReplayPublications() {
	for {
		// Wait for deferred work without polling or a growing request queue.
		select {
		case <-e.lifetime.Done():
			return
		case <-e.replayWake:
		}
		request := e.replayNext.Swap(nil)
		if request == nil {
			continue
		}
		store := e.so.GetBlockStore().(block.PublicationReserver)
		claim, err := store.ReservePublication(e.lifetime, request.bytes)
		if err != nil {
			if e.lifetime.Err() == nil {
				e.c.le.WithError(err).Warn("unable to reserve replay publication")
			}
			return
		}

		if claim == nil {
			return
		}

		// Replay afresh under the writer; prepareReplayPublication validates size.
		unlock, err := e.c.writeMtx.Lock(e.lifetime, "replay-publication")
		if err == nil && e.lifetime.Err() != nil {
			unlock()
			claim.Release()
			return
		}
		if err == nil {
			ctx := context.WithValue(e.lifetime, replayReservationKey{e}, claim)
			snap, readErr := e.admittedState(ctx)
			err = readErr
			if err == nil {
				_, err = e.advance(ctx, snap, nil)
			}
			unlock()
		}
		claim.Release()
		if err != nil && e.lifetime.Err() == nil && !isReplayGap(err) {
			e.c.le.WithError(err).Warn("unable to publish deferred replay")
		}
	}
}
