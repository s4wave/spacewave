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
// computation abandoned with its caller's context is claimed again by a
// waiter; another failure is retried without the cache. A part read while
// computing another value, from a snapshot the cache has moved past, is
// computed in that snapshot: the outer value is already shared, and opening
// the newest revision for each part costs more than reading it. A value
// computed at a revision the cache passed is kept for later readers when the
// changes since left its reads alone.
func (c *Cache) Get(ctx context.Context, tx world.WorldState, name string, compute func(context.Context, world.WorldState) (any, error)) (any, error) {
	seqno, err := tx.GetSeqno(ctx)
	if err != nil {
		return nil, err
	}
	if err := c.Advance(ctx, seqno); err != nil {
		return nil, err
	}

	for {
		// Reuse a value that holds at this revision, or claim its computation.
		c.mtx.Lock()
		entry := c.entries[name]
		if entry != nil && entry.at <= seqno && seqno <= c.seqno.Load() {
			c.mtx.Unlock()
			// A reader that leaves stops waiting, so streams a client
			// abandoned during a slow build do not pile up behind it.
			select {
			case <-entry.done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			if entry.err != nil {
				// A computation its own caller abandoned failed for that
				// caller alone: claim it again, so its waiters still share
				// one. Another failure is retried alone in this snapshot,
				// its parts read from tx rather than a transaction each.
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
		entry = &revisionEntry{name: name, done: make(chan struct{}), at: seqno, reads: world.NewReadSet(), blocks: block.NewRefHold()}
		c.keepLocked(entry)
		c.mtx.Unlock()

		// Compute in a child evaluation, then share its reads with this caller.
		child, finish := world.WithViewReads(block.WithRefHold(ctx, entry.blocks), entry.reads)
		child, parts := c.withParts(context.WithValue(child, computingKey{}, true))
		entry.value, entry.err = c.observe(name, path, func() (any, error) {
			return compute(child, entry.reads.Track(tx))
		})
		entry.wake = finish()
		if entry.err != nil {
			c.drop(name, entry)
			close(entry.done)
			return nil, entry.err
		}
		// Its parts are set under mtx, since a catch-up reads every entry's.
		c.mtx.Lock()
		entry.parts, entry.facets = parts.collected()
		c.mtx.Unlock()
		c.addViewReads(ctx, entry)

		// Check the value against the changes passed while it computed. One
		// they left alone holds at the newer revisions too, so the readers
		// that joined there share it instead of each starting another.
		c.mtx.Lock()
		if !c.heldSinceLocked(entry, seqno) {
			entry.stale = true
			if c.entries[name] == entry {
				delete(c.entries, name)
			}
			c.countDrop(name, "computing")
		}
		entry.finished = true
		if c.entries[name] == entry {
			c.indexLocked(entry)
		}
		close(entry.done)
		c.mtx.Unlock()
		return entry.value, entry.err
	}
}

// computeBehind computes the named value in tx, at seqno, a revision the cache
// has passed while it keeps another value under name that does not hold at
// seqno, and keeps it for later readers when that value is gone and the
// changes since seqno left its reads alone.
func (c *Cache) computeBehind(ctx context.Context, tx world.WorldState, seqno uint64, name string, compute func(context.Context, world.WorldState) (any, error)) (any, error) {
	// Compute in a child evaluation, then share its reads with this caller.
	reads, blocks := world.NewReadSet(), block.NewRefHold()
	child, finish := world.WithViewReads(block.WithRefHold(ctx, blocks), reads)
	child, parts := c.withParts(child)
	value, err := c.observe(name, "stale", func() (any, error) {
		return compute(child, reads.Track(tx))
	})
	wake := finish()
	if err != nil {
		return nil, err
	}

	// Publish the completed dependency set to this caller.
	plain, facets := parts.collected()
	entry := &revisionEntry{name: name, done: make(chan struct{}), at: seqno, value: value, reads: reads, blocks: blocks, wake: wake, parts: plain, facets: facets, finished: true}
	close(entry.done)
	c.addViewReads(ctx, entry)

	// Keep it when it still holds at the cache's revision.
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.entries[name] == nil && c.heldSinceLocked(entry, seqno) {
		c.keepLocked(entry)
	}
	return value, nil
}

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
