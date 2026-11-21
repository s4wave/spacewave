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
// reader behind the cache shares the value at the newest revision. A reader
// ahead of it never waits for the catch-up: it computes the value in its own
// snapshot, shared with the readers at the same revision. A
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
	// A reader ahead of the cache computes in its own snapshot rather than
	// wait for the catch-up, which waits on the World's change watch: upkeep
	// no read waits on. An empty cache moves to the reader's revision at once.
	if seqno > c.seqno.Load() && !c.jump(seqno) {
		c.catchUp(seqno)
		return c.computeApart(ctx, tx, seqno, name, compute)
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
				return c.computeApart(ctx, tx, seqno, name, compute)
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

// computeApart computes the named value in tx, at seqno, a revision other than
// the cache's: one the catch-up has not reached, or one it has passed while it
// keeps another value under name that does not hold at seqno. It keeps the
// value for later readers when no value is kept under name and the changes
// since seqno left its reads alone; a value ahead of the cache waits in ahead
// until the catch-up reaches seqno and can check that. The callers reading
// name at seqno share one computation: a reconnect wave opening snapshots ahead of
// the cache, or a burst of writes moving the head past many outer values
// computing at once, would otherwise have each compute the same value.
// A computation abandoned with its caller's context is claimed again by a
// waiter; another failure is retried alone by each waiter, in its own
// snapshot, as Get retries one, and is not kept.
func (c *Cache) computeApart(ctx context.Context, tx world.WorldState, seqno uint64, name string, compute func(context.Context, world.WorldState) (any, error)) (any, error) {
	key := apartKey{name: name, seqno: seqno}
	for {
		// Wait on the computation another caller claimed, or claim it. A
		// caller inside the claimed computation, reading the value from
		// itself through its parts, computes it alone instead of waiting
		// on itself.
		c.mtx.Lock()
		entry := c.apart[key]
		if entry != nil && ctx.Value(apartClaimKey{entry}) == nil {
			c.mtx.Unlock()
			select {
			case <-entry.done:
			case <-ctx.Done():
				return nil, ctx.Err()
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
		entry = &revisionEntry{name: name, done: make(chan struct{}), at: seqno, reads: world.NewReadSet(), blocks: block.NewRefHold()}
		claimed := c.apart[key] == nil
		if claimed {
			if c.apart == nil {
				c.apart = make(map[apartKey]*revisionEntry)
			}
			c.apart[key] = entry
		}
		c.mtx.Unlock()

		// Compute in a child evaluation, then share its reads with this caller.
		child, finish := world.WithViewReads(block.WithRefHold(ctx, entry.blocks), entry.reads)
		child, parts := c.withParts(context.WithValue(child, apartClaimKey{entry}, true))
		path := "stale"
		if seqno > c.seqno.Load() {
			path = "ahead"
		}
		entry.value, entry.err = c.observe(name, path, func() (any, error) {
			return compute(child, entry.reads.Track(tx))
		})
		entry.wake = finish()
		if entry.err == nil {
			entry.parts, entry.facets = parts.collected()
			entry.finished = true
		}

		// End the claim, and keep the value when it holds at the cache's
		// revision, or hold it until the catch-up reaches its own.
		c.mtx.Lock()
		if claimed && c.apart[key] == entry {
			delete(c.apart, key)
		}
		if entry.err == nil {
			c.keepApartLocked(entry)
		}
		close(entry.done)
		c.mtx.Unlock()
		if entry.err != nil {
			return nil, entry.err
		}

		// Publish the completed dependency set to this caller.
		c.addViewReads(ctx, entry)
		return entry.value, nil
	}
}

// keepApartLocked keeps entry, a finished value computed at a revision other
// than the cache's, when no value is kept under its name and the changes the
// cache passed since its revision left it alone. A value past the cache's
// revision replaces an older one waiting in ahead, until adoptAheadLocked
// checks it. The caller holds mtx.
func (c *Cache) keepApartLocked(entry *revisionEntry) {
	// Hold a value ahead of the cache until the catch-up reaches it.
	if entry.at > c.seqno.Load() {
		if old := c.ahead[entry.name]; old == nil || old.at < entry.at {
			if c.ahead == nil {
				c.ahead = make(map[string]*revisionEntry)
			}
			c.ahead[entry.name] = entry
		}
		return
	}

	// Keep a value the changes since its revision left alone.
	if c.entries[entry.name] == nil && c.heldSinceLocked(entry, entry.at) {
		c.keepLocked(entry)
	}
}

// adoptAheadLocked checks each value waiting in ahead that the cache has
// reached, keeping it as keepApartLocked does. The caller holds mtx, and calls
// it after each move of seqno.
func (c *Cache) adoptAheadLocked() {
	seqno := c.seqno.Load()
	for name, entry := range c.ahead {
		if entry.at <= seqno {
			delete(c.ahead, name)
			c.keepApartLocked(entry)
		}
	}
}

// apartKey names the computation of a value at a revision other than the
// cache's.
type apartKey struct {
	name  string
	seqno uint64
}

// apartClaimKey marks a context computing the claimed value entry apart from
// the cache's revision.
type apartClaimKey struct{ entry *revisionEntry }

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
// cache moved past, "ahead" for a value computed in a snapshot the cache has
// not reached, and "retry" for a failed value retried alone.
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
