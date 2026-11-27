package viewcache

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// Get returns the named value at tx's revision, computing it at most once
// while the World leaves its reads untouched. A view evaluation in ctx depends
// on everything the value read, whether or not this call computed it. A
// reader behind the cache shares the value at the newest revision. A
// computation runs on a context the cache owns and ends early only when
// every caller waiting on it has left. A reader that leaves returns its own
// context error at once, except the computing caller, which waits for the
// computation to end because it reads that caller's snapshot. The next
// reader claims an abandoned computation again. Another failure is retried
// without the cache.
//
// A part read while computing another value, from a snapshot the cache has
// moved past, is computed in that snapshot. The outer value is already shared,
// and opening the newest revision for each part costs more than reading it.
// A value computed at a revision the cache passed is kept for later readers
// when the changes since left its reads alone. A reader whose ctx carries a floor
// (WithFloor) takes the name's last built value at or past the floor instead
// of waiting on a build that a change passed, as Recent answers it.
func (c *Cache) Get(ctx context.Context, tx world.WorldState, name string, compute func(context.Context, world.WorldState) (any, error)) (any, error) {
	// Reach the snapshot revision before choosing or claiming a value.
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.Advance(ctx, seqno); err != nil {
		return nil, err
	}

	// Retry a claim when the computation was abandoned or its result went stale.
	for {
		// Reuse a value that holds at this revision, or claim its computation.
		c.mtx.Lock()
		entry := c.entries[name]
		if entry != nil && entry.at <= seqno && seqno <= c.seqno.Load() {
			c.mtx.Unlock()
			// A reader that leaves stops waiting, so streams a client
			// abandoned during a slow build do not pile up behind it.
			if err := entry.await(ctx); err != nil {
				return nil, err
			}
			if entry.err != nil {
				// Claim an abandoned computation again so its waiters still
				// share one. Retry another failure alone in this snapshot,
				// reading each part from tx.
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if errors.Is(entry.err, context.Canceled) || errors.Is(entry.err, context.DeadlineExceeded) {
					continue
				}
				return c.observe(name, "retry", func() (any, error) {
					return compute(context.WithValue(ctx, computingKey{}, true), tx)
				})
			}
			if entry.stale && seqno != entry.at {
				// A reader that accepts the last built value takes it
				// rather than wait on another build.
				if value, _, ok := c.lastBuilt(ctx, name, compute); ok {
					return value, nil
				}
				continue
			}
			if entry.expired() {
				c.drop(name, entry)
				continue
			}
			c.addViewReads(ctx, entry)
			return entry.value, nil
		}

		// A reader behind the cache reads at the head, but a part of a value
		// being computed is read in that value's snapshot. While no value is
		// kept, it claims the entry like one at the head, so the parts many
		// values computed behind share are each computed once.
		path := "shared"
		if seqno != c.seqno.Load() {
			if !computing(ctx) {
				c.mtx.Unlock()
				return c.getAtHead(ctx, name, compute)
			}
			if entry != nil {
				c.mtx.Unlock()
				return c.computeBehind(ctx, tx, seqno, name, compute)
			}
			path = "stale"
		}

		// Keep the claim visible while its computation runs without the lock.
		entry = newRevisionEntry(name, seqno, block.ColdReadGateOf(ctx))
		owned, release := entry.own(ctx)
		c.keepLocked(entry)
		c.mtx.Unlock()

		// Compute in a child evaluation, then share its reads with this caller.
		var parts *entryParts
		err := entry.evaluate(owned, world.Untracked(tx), func(child context.Context, tracked world.WorldState) (any, error) {
			// Collect shared dependencies apart from the entry's direct reads.
			child, parts = c.withParts(context.WithValue(child, computingKey{}, true))
			return c.observe(name, path, func() (any, error) {
				return compute(child, tracked)
			})
		})
		release()
		if err != nil {
			c.drop(name, entry)
			entry.signalDone()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, err
		}

		// The entry's parts are set under mtx, since a catch-up reads
		// every entry's parts.
		c.mtx.Lock()
		entry.collectParts(parts.collected)
		c.mtx.Unlock()

		// Check the value against the changes passed while it computed.
		// A value the changes left alone holds at the newer revisions
		// too, so the readers that joined there share it instead of
		// each starting another.
		c.mtx.Lock()
		if !c.heldSinceLocked(entry, seqno) {
			entry.markStale()
			if c.entries[name] == entry {
				c.removeLocked(name)
			}
			c.keepBuiltLocked(entry)
			c.countDrop(name, "computing")
		}
		entry.finishComputation()
		if c.entries[name] == entry {
			c.indexLocked(entry)
		}
		entry.signalDone()
		c.mtx.Unlock()

		// Share the completed dependency set with a caller still waiting.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		c.addViewReads(ctx, entry)
		return entry.value, entry.err
	}
}

// computeBehind computes the named value in tx, at seqno, a revision the cache
// has passed while it keeps another value under name that does not hold at
// seqno, and keeps it for later readers when that value is gone and the
// changes since seqno left its reads alone. The callers reading name at seqno
// share one computation: a burst of writes moving the head past many outer
// values computing at once would otherwise have each compute the same part.
// As in Get, the cache owns the computation's context and cancels it once
// every caller has left. A reader that leaves returns its own context error
// at once, except the computing caller, which waits for the computation to
// finish because it reads that caller's snapshot. The next reader claims an
// abandoned computation again. Each waiter retries another failure alone in
// its own snapshot, without keeping the result.
func (c *Cache) computeBehind(
	ctx context.Context,
	tx world.WorldState,
	seqno uint64,
	name string,
	compute func(context.Context, world.WorldState) (any, error),
) (any, error) {
	// Identify the snapshot whose callers share one behind computation.
	key := behindKey{name: name, seqno: seqno}

	// Retry the claim only when the computation was abandoned.
	for {
		// Wait on the computation another caller claimed, or claim it. A
		// caller inside the claimed computation, reading the value from
		// itself through its parts, computes it alone instead of waiting
		// on itself.
		c.mtx.Lock()
		entry := c.behind[key]
		if entry != nil && ctx.Value(behindClaimKey{entry}) == nil {
			c.mtx.Unlock()
			if err := entry.await(ctx); err != nil {
				return nil, err
			}
			if entry.err != nil {
				if err := ctx.Err(); err != nil {
					return nil, err
				}
				if errors.Is(entry.err, context.Canceled) || errors.Is(entry.err, context.DeadlineExceeded) {
					continue
				}
				return c.observe(name, "retry", func() (any, error) {
					return compute(ctx, tx)
				})
			}
			c.addViewReads(ctx, entry)
			return entry.value, nil
		}

		// Publish a claim only when no other caller owns this snapshot.
		entry = newRevisionEntry(name, seqno, block.ColdReadGateOf(ctx))
		owned, release := entry.own(ctx)
		claimed := c.behind[key] == nil
		if claimed {
			if c.behind == nil {
				c.behind = make(map[behindKey]*revisionEntry)
			}
			c.behind[key] = entry
		}
		c.mtx.Unlock()

		// Compute in a child evaluation, then share its reads with this caller.
		var parts *entryParts
		err := entry.evaluate(owned, world.Untracked(tx), func(child context.Context, tracked world.WorldState) (any, error) {
			// Collect shared dependencies apart from the entry's direct reads.
			child, parts = c.withParts(context.WithValue(child, behindClaimKey{entry}, true))
			return c.observe(name, "stale", func() (any, error) {
				return compute(child, tracked)
			})
		})
		release()
		if err == nil {
			entry.collectParts(parts.collected)
			entry.finishComputation()
		}

		// End the claim, and keep the value when it still holds at the
		// cache's revision.
		c.mtx.Lock()
		if claimed && c.behind[key] == entry {
			delete(c.behind, key)
		}
		if entry.err == nil && c.entries[name] == nil && c.heldSinceLocked(entry, seqno) {
			c.keepLocked(entry)
		}
		entry.signalDone()
		c.mtx.Unlock()

		// Return this caller's cancellation after publishing the shared result.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if entry.err != nil {
			return nil, entry.err
		}

		// Publish the completed dependency set to this caller.
		c.addViewReads(ctx, entry)
		return entry.value, nil
	}
}

// behindKey identifies the computation of a value at a revision the cache
// passed.
type behindKey struct {
	name  string
	seqno uint64
}

// behindClaimKey marks a context computing the claimed value entry behind the
// cache.
type behindClaimKey struct{ entry *revisionEntry }

// heldSinceLocked reports whether the changes the cache passed after seqno
// left entry alone. It reports false when the history no longer reaches back
// to seqno. The caller holds mtx.
func (c *Cache) heldSinceLocked(entry *revisionEntry, seqno uint64) bool {
	if seqno >= c.seqno.Load() {
		return true
	}
	if len(c.history) == 0 || c.history[0].from > seqno {
		return false
	}
	for _, step := range c.history {
		if step.through > seqno && entry.touched(step.changes, nil) {
			return false
		}
	}
	return true
}

// observe runs compute, counting it under name's family and path: "shared"
// for a value the cache keeps, "stale" for a part computed in a snapshot the
// cache moved past, and "retry" for a failed value retried alone.
func (c *Cache) observe(name, path string, compute func() (any, error)) (any, error) {
	// Report the completed computation only when an observer is configured.
	if c.observer == nil {
		return compute()
	}
	family, _, _ := strings.Cut(name, ":")
	started := time.Now()
	value, err := compute()
	c.observer.Computed(family, path, time.Since(started))
	return value, err
}

// countDrop counts a value a catch-up dropped for cause: "changed" when a
// change touched its reads, "unknown" when the changes could not say what
// changed, or "computing" for one the changes passed while it computed touched.
func (c *Cache) countDrop(name, cause string) {
	if c.observer == nil {
		return
	}
	family, _, _ := strings.Cut(name, ":")
	c.observer.Dropped(family, cause)
}

// computingKey marks a context computing a revision cache value.
type computingKey struct{}

// computing reports whether ctx computes a revision cache value.
func computing(ctx context.Context) bool {
	return ctx.Value(computingKey{}) != nil
}

// Value is c.get for a value of type T. A nil c computes the value in
// tx without sharing it, as a write transaction must.
func Value[T any](ctx context.Context, c *Cache, tx world.WorldState, name string, compute func(context.Context, world.WorldState) (T, error)) (T, error) {
	if c == nil {
		return compute(ctx, tx)
	}
	value, err := c.Get(ctx, tx, name, func(ctx context.Context, tx world.WorldState) (any, error) {
		return compute(ctx, tx)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return value.(T), nil
}
