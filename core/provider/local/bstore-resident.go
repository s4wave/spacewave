package provider_local

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_fillcache "github.com/s4wave/spacewave/db/block/store/fillcache"
	"github.com/sirupsen/logrus"
)

// residentSaveInterval is the time between two saves of a mount's resident
// list, so a crash loses at most this much of the list.
const residentSaveInterval = 5 * time.Minute

// residentSaveWait bounds the save at unmount, which delays the unmount.
const residentSaveWait = 10 * time.Second

// residentWaitBytes is the bytes the warm-up queues into the fill cache before
// it waits for the cache to write them. It keeps the warm-up far below the
// cache's queue bound, so a foreground fill queued beside it is never dropped.
const residentWaitBytes = 16 << 20

// residentWarmup turns the resident list's save and warm-up on; see
// SetResidentWarmup.
var residentWarmup atomic.Bool

func init() {
	residentWarmup.Store(true)
}

// SetResidentWarmup turns on or off, for the block stores mounted after the
// call, the list of blocks each mount's decoded-block cache held and the
// warm-up that reads them into the fill cache at the next mount. Off, a mount
// neither saves nor reads its list.
func SetResidentWarmup(on bool) {
	residentWarmup.Store(on)
}

// residentCache is a fill cache that keeps a resident list per block store.
type residentCache interface {
	// SaveResident replaces scope's list with refKeys.
	SaveResident(ctx context.Context, scope string, refKeys iter.Seq[string]) error
	// MissingResident calls fn with the listed refs the cache lacks, and
	// returns the number the list named.
	MissingResident(ctx context.Context, scope string, fn func([]*block.BlockRef) error) (int, error)
	// Fill queues a block to be copied into the cache.
	Fill(ctx context.Context, primary block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error
	// Wait waits until the cache wrote what was queued.
	Wait(ctx context.Context) error
}

// batchReader is a backend store that reads a batch of blocks with few
// requests.
type batchReader interface {
	// ReadBatch calls fn with each block of refs it reads, skipping any.
	ReadBatch(ctx context.Context, refs []*block.BlockRef, fn func(*block.BlockRef, *block.StoredBlock) error) error
}

// startResident keeps the mount's resident list in the fill cache, saving it
// every residentSaveInterval and at the returned stop, and reads the list the
// last mount saved into the cache in the background. The caller calls stop
// after ctx ends and before decodedBlocks closes, so the last save walks the
// scope this mount filled. Without a fill cache that keeps lists, or with the
// warm-up off, it does nothing.
//
// The list is a hint the warm-up reads through the backend, which verifies
// every block, and fills only the cache, which holds copies alone: a stale or
// lost list costs reads and never a block.
func (t *bstoreTracker) startResident(ctx context.Context, le *logrus.Entry, p *placementState, decodedBlocks *block.DecodedBlockCache) (stop func()) {
	// Keep lists only in a fill cache that can, with the warm-up on.
	cache, ok := p.cache.(residentCache)
	if !ok || !residentWarmup.Load() {
		return func() {}
	}

	// A save walks the scope's live entries into the list.
	le = le.WithField("routine", "resident-list")
	save := func(ctx context.Context) {
		keys := func(yield func(string) bool) {
			for r := range decodedBlocks.Resident() {
				if !yield(r.RefKey) {
					return
				}
			}
		}
		if err := cache.SaveResident(ctx, t.id, keys); err != nil {
			le.WithError(err).Warn("unable to save the resident list")
		}
	}

	// Warm the cache and save the list every interval until ctx ends.
	var wg sync.WaitGroup
	wg.Go(func() { t.warmResident(ctx, le, p, cache) })
	wg.Go(func() {
		// The interval bounds what a crash loses of the list; it polls no state.
		//gostyle:timer deadline
		ticker := time.NewTicker(residentSaveInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				save(ctx)
			}
		}
	})

	// At stop, wait for both and save the list a last time.
	return func() {
		wg.Wait()
		saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), residentSaveWait)
		defer cancel()
		save(saveCtx)
	}
}

// warmResident reads the blocks the last mount's list names and the cache
// lacks from the backend into the cache, once the backend opens. It reads a
// batch at a time and waits for the cache after every residentWaitBytes, and
// stops at the first failed batch: the blocks it did not read read on demand.
func (t *bstoreTracker) warmResident(ctx context.Context, le *logrus.Entry, p *placementState, cache residentCache) {
	// Wait for the backend to open, and warm only from one that reads batches.
	store, err := t.waitBackend(ctx, p)
	if err != nil || store == nil {
		return
	}
	batch, ok := store.(batchReader)
	if !ok {
		return
	}

	// Read each slice of missing refs into the cache.
	start := time.Now()
	var missing, fetched int
	var queued int64
	listed, err := cache.MissingResident(ctx, t.id, func(refs []*block.BlockRef) error {
		missing += len(refs)
		return batch.ReadBatch(ctx, refs, func(ref *block.BlockRef, stored *block.StoredBlock) error {
			// Queue the block, and let the cache write each 16 MiB queued.
			if err := cache.Fill(ctx, nil, ref, stored); err != nil {
				return err
			}
			fetched++
			queued += int64(len(stored.Data))
			if queued < residentWaitBytes {
				return nil
			}
			queued = 0
			return cache.Wait(ctx)
		})
	})
	le = le.WithField("listed", listed).
		WithField("held", listed-missing).
		WithField("fetched", fetched).
		WithField("dur", time.Since(start).Round(time.Millisecond).String())
	if err != nil && ctx.Err() == nil {
		le.WithError(err).Warn("resident warm-up stopped")
		return
	}
	if listed != 0 {
		le.Info("resident warm-up done")
	}
}

// waitBackend returns the placement's open backend store once it opens, or
// nil once the mount is unplaced or ctx ends.
func (t *bstoreTracker) waitBackend(ctx context.Context, p *placementState) (block.StoreOps, error) {
	for {
		store, changed := t.readBackend(p)
		if store != nil || changed == nil {
			return store, nil
		}
		select {
		case <-ctx.Done():
			return nil, context.Cause(ctx)
		case <-changed:
		}
	}
}

// _ is a type assertion: the disk fill cache keeps resident lists.
var _ residentCache = (*block_store_fillcache.Cache)(nil)
