package provider_local

import (
	"context"
	"iter"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_fillcache "github.com/s4wave/spacewave/db/block/store/fillcache"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/sirupsen/logrus"
)

// residentSaveInterval is the time between two saves of a mount's resident
// list, so a crash loses at most this much of the list.
const residentSaveInterval = 5 * time.Minute

// residentSaveWait bounds the save at unmount, which delays the unmount.
const residentSaveWait = 10 * time.Second

// warmRetryFirst and warmRetryMax bound the warm-up's wait before it reads a
// slice the bucket failed with a transient status again: the wait starts at
// the first and doubles up to the second.
const (
	warmRetryFirst = 5 * time.Second
	warmRetryMax   = time.Minute
)

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

// ResidentCache is a fill cache that keeps a resident list per block store.
// A mount keeps its list only when its placement's fill cache implements it,
// so a cache that wraps a fill cache implements it to forward the list.
type ResidentCache interface {
	// SaveResident replaces scope's list with refKeys.
	SaveResident(ctx context.Context, scope string, refKeys iter.Seq[string]) error
	// WarmResident reads the listed refs the cache lacks with read into the
	// cache.
	WarmResident(ctx context.Context, scope string, read block_store_fillcache.ReadBatch) (block_store_fillcache.WarmCounts, error)
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
	cache, ok := p.cache.(ResidentCache)
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

// warmResident reads the blocks the last mount's list names and neither the
// cache nor the local store holds from the backend into the cache, once the backend opens, when the
// backend reads batches. The blocks it does not read read on demand.
func (t *bstoreTracker) warmResident(ctx context.Context, le *logrus.Entry, p *placementState, cache ResidentCache) {
	// Wait for the backend to open, and warm only from one that reads batches.
	store, err := t.waitBackend(ctx, p)
	if err != nil || store == nil {
		return
	}
	batch, ok := store.(batchReader)
	if !ok {
		return
	}

	// Read the list into the cache, skipping what the local store serves, and
	// log what it did.
	start := time.Now()
	var local, retries int
	fetch := retryTransient(le, batch.ReadBatch, warmRetryFirst, &retries)
	read := skipLocal(&heldReads{StoreOps: p.local, held: &t.held}, fetch, &local)
	counts, err := cache.WarmResident(ctx, t.id, read)
	le = le.WithField("listed", counts.Listed).
		WithField("held", counts.Listed-counts.Missing).
		WithField("local", local).
		WithField("fetched", counts.Fetched).
		WithField("retries", retries).
		WithField("dur", time.Since(start).Round(time.Millisecond).String())
	if err != nil && ctx.Err() == nil {
		le.WithError(err).Warn("resident warm-up stopped")
		return
	}
	if counts.Listed != 0 {
		le.Info("resident warm-up done")
	}
}

// skipLocal returns a ReadBatch that reads with read only the refs local lacks,
// adding the count it skipped to skipped. A block local holds is served from
// local on a cache miss, so fetching it from the bucket would cost a bucket
// read to save a local one; the list names such blocks, as the decoded cache
// sits above both.
// Each call costs one batched existence probe of local. A failed probe fails
// the call, ending the warm-up, rather than reading the whole slice from the
// bucket.
func skipLocal(local block.StoreOps, read block_store_fillcache.ReadBatch, skipped *int) block_store_fillcache.ReadBatch {
	var want []*block.BlockRef
	return func(ctx context.Context, refs []*block.BlockRef, fn func(*block.BlockRef, *block.StoredBlock) error) error {
		// Keep the refs local lacks.
		held, err := local.GetBlockExistsBatch(ctx, refs)
		if err != nil {
			return err
		}
		want = want[:0]
		for i, ref := range refs {
			if !held[i] {
				want = append(want, ref)
			}
		}
		*skipped += len(refs) - len(want)

		// Read them.
		if len(want) == 0 {
			return nil
		}
		return read(ctx, want, fn)
	}
}

// retryTransient returns a ReadBatch that reads with read and, when the bucket
// fails a read with a transient status, such as a throttled account's 503
// SlowDown, waits and reads again the refs it has not yet passed to fn. The
// wait starts at first and doubles up to warmRetryMax, and retries counts the
// waits. Other errors return.
//
// The bucket's client has retried the request before it returns the error, so
// the bucket is throttling for longer than a request's retries cover, as it
// does when a start's own fills pass the account's rate. The warm-up is
// background work that a start can finish without, so it waits out the fault
// rather than adding to the burst or ending: one throttled minute delays it.
// Each wait costs one timer, and a retry reads only the refs still unread.
func retryTransient(le *logrus.Entry, read block_store_fillcache.ReadBatch, first time.Duration, retries *int) block_store_fillcache.ReadBatch {
	return func(ctx context.Context, refs []*block.BlockRef, fn func(*block.BlockRef, *block.StoredBlock) error) error {
		// Read, recording each ref passed on, until a read ends without a
		// transient failure.
		var passed map[string]struct{}
		wait := first
		for {
			err := read(ctx, refs, func(ref *block.BlockRef, stored *block.StoredBlock) error {
				if passed == nil {
					passed = make(map[string]struct{}, len(refs))
				}
				passed[ref.MarshalString()] = struct{}{}
				return fn(ref, stored)
			})
			if err == nil || ctx.Err() != nil || !block_store_s3.IsTransient(err) {
				return err
			}

			// Keep the refs still unread, then wait.
			refs = slices.DeleteFunc(slices.Clone(refs), func(ref *block.BlockRef) bool {
				_, ok := passed[ref.MarshalString()]
				return ok
			})
			if len(refs) == 0 {
				return nil
			}
			*retries++
			le.WithError(err).WithField("wait", wait.String()).Debug("the bucket failed the resident warm-up, waiting to retry")
			// The wait backs off a throttled bucket; it polls no state.
			//gostyle:timer deadline
			timer := time.NewTimer(wait)
			select {
			case <-ctx.Done():
				timer.Stop()
				return context.Cause(ctx)
			case <-timer.C:
			}
			wait = min(2*wait, warmRetryMax)
		}
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
var _ ResidentCache = (*block_store_fillcache.Cache)(nil)
