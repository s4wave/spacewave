package sobject_world_engine

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// reservePublication keeps the independent candidate and its staging ownership
// while returning the writer lock to replay and other writes. The original
// predecessor is checked after reacquisition: a failed base is never replayed
// into a success. The current replay save is rebuilt under the lock, and a
// larger save returns its claim and retries outside the lock. No queue wait
// holds either the engine's writer or the provider's state writer.
func (t *soEngineWriteTx) reservePublication(ctx context.Context, publisher sobject.StatePublisher) error {
	// Unadmitted work ends with either the caller or the retired engine.
	ctx, cancel := t.eng.publicationContext(ctx)
	defer cancel()

	// Placements without a reservation lane retain their existing commit path.
	store, ok := t.eng.so.GetBlockStore().(block.PublicationReserver)
	if !ok || !store.SupportsPublicationReservations() {
		return nil
	}

	// Retain the candidate's immutable batch and original predecessor across waits.
	if t.prepared == nil {
		batch, err := t.btx.TakePending(ctx)
		if err != nil {
			return err
		}
		t.prepared = batch
	}
	after := t.baseReceipt

	// Retry a claim when replay's save grew while another writer ran.
	for {
		// Size the current save while replay is held, before releasing the writer.
		size, err := t.publicationBytes(ctx, publisher)
		if err != nil {
			return err
		}
		reservation, err := store.TryReservePublication(ctx, size)
		if err != nil {
			return err
		}
		if reservation != nil {
			t.reservation = reservation
			return nil
		}

		// Only a full queue opens the candidate's independent-fork interval.
		t.unlockWriteMtx()
		reservation, err = store.ReservePublication(ctx, size)
		if err != nil {
			return err
		}

		// Reject a failed original base before competing with its reinstall.
		if after != nil && after.Err() != nil {
			if reservation != nil {
				reservation.Release()
			}
			return block.NewPublicationDependencyError(after.Err())
		}

		// Reacquire only the writer: queueOperation places a stale fork through replay.
		unlock, err := t.eng.c.writeMtx.Lock(ctx, "publication-reservation")
		if err != nil {
			if reservation != nil {
				reservation.Release()
			}
			return err
		}
		t.unlockWriteMtx = sync.OnceFunc(unlock)
		if t.eng.lifetime.Err() != nil {
			if reservation != nil {
				reservation.Release()
			}
			return world_block.ErrEngineClosed
		}
		t.eng.settle()
		if after != nil && after.Err() != nil {
			if reservation != nil {
				reservation.Release()
			}
			return block.NewPublicationDependencyError(after.Err())
		}

		// Replay may have advanced while capacity was unavailable. Its new save
		// must fit this claim; otherwise reserve its new size without the lock.
		current, err := t.publicationBytes(ctx, publisher)
		if err != nil {
			if reservation != nil {
				reservation.Release()
			}
			return err
		}
		if reservation == nil || current <= reservation.Bytes() {
			t.reservation = reservation
			return nil
		}
		reservation.Release()
	}
}

// publicationContext ends pre-admission waits with the caller or the engine.
func (e *soEngine) publicationContext(ctx context.Context) (context.Context, context.CancelFunc) {
	bound, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(e.lifetime, cancel)
	return bound, func() { stop(); cancel() }
}

// publicationBytes bounds SQL-primary payloads before the state writer is taken.
// It counts every candidate page even when the page view may keep some of them.
func (t *soEngineWriteTx) publicationBytes(ctx context.Context, publisher sobject.StatePublisher) (int64, error) {
	// Retain the exact save the publication will consume under this writer hold.
	entries, roots, cursor, err := t.eng.replay.pendingSave(ctx, publisher, t.fork.payloads)
	if err != nil {
		return 0, err
	}
	t.preparedSave, t.preparedRoots, t.preparedCursor = entries, roots, cursor

	// Charge the candidate and save payloads once per publication attempt.
	var size int64
	for _, entry := range t.prepared.Entries {
		size += int64(len(entry.Data))
	}
	for _, entry := range entries {
		size += int64(len(entry.Data))
	}
	return size, nil
}

// reserveStateWrite takes state intent without waiting under the engine writer.
// User callbacks run before this claim, so they may themselves change metadata.
func (t *soEngineWriteTx) reserveStateWrite(ctx context.Context, settled bool) (context.Context, error) {
	// Only reservation-enabled stores need the shared intent lane.
	reserver, ok := t.eng.so.(sobject.StateWriteReserver)
	store, reserves := t.eng.so.GetBlockStore().(block.PublicationReserver)
	if !ok || !reserves || !store.SupportsPublicationReservations() {
		return ctx, nil
	}
	bound, cancel := t.eng.publicationContext(ctx)
	defer cancel()

	// A fast claim keeps the usual commit under its existing engine hold.
	var reservation sobject.StateWriteReservation
	var err error
	if !settled {
		reservation, err = reserver.TryReserveStateWrite(bound)
	}
	if err != nil {
		return ctx, err
	}
	if reservation == nil {
		t.unlockWriteMtx()
		reservation, err = reserver.ReserveStateWrite(bound, settled)
		if err != nil {
			return ctx, err
		}
		unlock, err := t.eng.c.writeMtx.Lock(bound, "state-reservation")
		if err != nil {
			if reservation != nil {
				reservation.Release()
			}
			return ctx, err
		}
		t.unlockWriteMtx = sync.OnceFunc(unlock)
		if t.eng.lifetime.Err() != nil {
			if reservation != nil {
				reservation.Release()
			}
			return ctx, world_block.ErrEngineClosed
		}
		t.eng.settle()
		if t.baseReceipt != nil && t.baseReceipt.Err() != nil {
			if reservation != nil {
				reservation.Release()
			}
			return ctx, block.NewPublicationDependencyError(t.baseReceipt.Err())
		}
	}
	t.stateReservation, t.stateSettled = reservation, settled
	if reservation != nil {
		ctx = reservation.WithContext(ctx)
	}
	return ctx, nil
}

// releaseStateWrite returns commit intent after admission or cancellation.
func (t *soEngineWriteTx) releaseStateWrite() {
	if t.stateReservation != nil {
		t.stateReservation.Release()
		t.stateReservation = nil
	}
}
