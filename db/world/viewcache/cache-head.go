package viewcache

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// getAtHead returns the named value at the World's newest revision, for a
// reader whose snapshot the cache has moved past. Every such reader then
// shares one computation instead of each repeating it alone; the reads it
// records make the reader's watch rerun at the newer revision.
func (c *Cache) getAtHead(ctx context.Context, name string, compute func(context.Context, world.WorldState) (any, error)) (any, error) {
	head, err := c.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer head.Discard()
	return c.Get(ctx, head, name, compute)
}

// backgroundReadTimeout bounds a catch-up or refresh the cache runs in the
// background, which no caller's context ends.
const backgroundReadTimeout = time.Minute

// Head returns a revision at or past floor at which the cache's values hold,
// so a reader can look them up there without opening a snapshot. When the
// cache stands at or past floor it answers there at once: a read never waits
// on the change watch for a revision it did not ask for. Only a reader that
// needs a revision the cache has not reached, such as a player reading after
// their own write, waits for the catch-up.
//
// While the change watch feeds the cache (follow), head reads two atomics and
// takes no lock: it runs on every cached read, hundreds of thousands a second,
// and one shared mutex there serializes them all.
func (c *Cache) Head(ctx context.Context, floor uint64) (uint64, error) {
	// Answer a retained revision that meets the caller's floor.
	newest, err := c.NewestKnown(ctx)
	if err != nil {
		return 0, err
	}
	cached := c.seqno.Load()
	if cached != 0 && cached >= floor {
		c.observeHead(newest-min(newest, cached), "")
		return cached, nil
	}
	cause := "floor"
	if cached == 0 {
		cause = "empty"
	}
	c.observeHead(0, cause)

	// The watch may not have reported the floor yet, as for the reader's own
	// write a moment ago; the catch-up waits for the World to reach it.
	target := max(newest, floor)
	return target, c.Advance(ctx, target)
}

// NewestKnown returns the World's newest revision as the change watch last
// reported it, or, while no watch feeds the cache, as the World answers, and
// then starts the catch-up to it that the watch would have started.
func (c *Cache) NewestKnown(ctx context.Context) (uint64, error) {
	// Use the driving watch or ask the World and start catch-up.
	if watched := c.watched.Load(); watched != 0 {
		return watched, nil
	}
	newest, err := c.engine.GetSeqno(ctx)
	if err != nil {
		return 0, err
	}
	if newest > c.seqno.Load() {
		c.catchUp(newest)
	}
	return newest, nil
}

// observeHead records a head answer lag revisions behind the World's newest,
// and the wait's cause when it waited for a catch-up.
func (c *Cache) observeHead(lag uint64, wait string) {
	if c.observer != nil {
		c.observer.HeadRead(lag, wait)
	}
}

// Newest brings the cache to the World's newest revision and returns it, for
// a reader that needs the published head itself.
func (c *Cache) Newest(ctx context.Context) (uint64, error) {
	seqno, err := c.engine.GetSeqno(ctx)
	if err != nil {
		return 0, err
	}
	return seqno, c.Advance(ctx, seqno)
}

// Follow records seqno, a revision the World's change watch reported, as the
// newest, and catches the cache up to it in the background. From the first
// call head takes the newest revision from the watch instead of asking the
// World on every read. unfollow ends that once the watch stops.
func (c *Cache) Follow(seqno uint64) {
	raise(&c.watched, seqno)
	c.catchUp(seqno)
}

// Unfollow returns head to asking the World for its newest revision, once the
// change watch that called follow has stopped.
func (c *Cache) Unfollow() {
	c.watched.Store(0)
}

// catchUp brings the cache to revision seqno in the background. Exactly one
// catch-up runs at a time: a revision that arrives while one runs only raises
// target, and the running one advances to it next, so nobody starts a second.
func (c *Cache) catchUp(seqno uint64) {
	raise(&c.target, seqno)
	if !c.catchingUp.Load() && c.catchingUp.CompareAndSwap(false, true) {
		go c.runCatchUp()
	}
}

// runCatchUp advances the cache until it reaches target, then clears
// catchingUp. A failed advance ends the run without retrying, so an error that
// persists, such as a closing engine, never loops hot; the next catchUp, from
// the next revision the watch reports or the next read, starts again.
func (c *Cache) runCatchUp() {
	for {
		for target := c.target.Load(); target > c.seqno.Load(); target = c.target.Load() {
			ctx, cancel := context.WithTimeout(context.Background(), backgroundReadTimeout)
			err := c.Advance(ctx, target)
			cancel()
			if err != nil {
				logrus.WithError(err).WithField("target", target).Debug("revision cache catch-up failed")
				c.catchingUp.Store(false)
				return
			}
		}
		c.catchingUp.Store(false)

		// A target raised between the last check and the store above found
		// the run still marked and started none; take it up here.
		if c.target.Load() <= c.seqno.Load() || !c.catchingUp.CompareAndSwap(false, true) {
			return
		}
	}
}

// raise sets v to seqno when seqno is newer.
func raise(v *atomic.Uint64, seqno uint64) {
	for old := v.Load(); seqno > old && !v.CompareAndSwap(old, seqno); old = v.Load() {
	}
}

// Peek returns the named value when the cache holds it, finished, at seqno,
// and adds what it read to the view evaluation in ctx as get does. ok is
// false when the value must be computed, waited on or rechecked: the reader
// then opens a snapshot and asks get.
func (c *Cache) Peek(ctx context.Context, seqno uint64, name string) (any, bool) {
	// Reuse a finished value without waiting for a computation.
	c.mtx.Lock()
	entry := c.entries[name]
	held := entry != nil && entry.at <= seqno && seqno <= c.seqno.Load()
	c.mtx.Unlock()
	if !held {
		return nil, false
	}
	return c.finishedValue(ctx, entry, seqno)
}

// finishedValue returns entry's value when it is finished, current at seqno
// and unexpired, and adds what it read to the view evaluation in ctx.
func (c *Cache) finishedValue(ctx context.Context, entry *revisionEntry, seqno uint64) (any, bool) {
	select {
	case <-entry.done:
	default:
		return nil, false
	}
	if entry.err != nil || (entry.stale && seqno != entry.at) || entry.expired() {
		return nil, false
	}
	c.addViewReads(ctx, entry)
	return entry.value, true
}

// Recent returns the named value at seqno, a revision head returned for
// floor, and the revision it holds at. When the cache has no finished value
// there, as after a catch-up dropped it, it answers the dropped value at the
// Newest revision it is known to hold at, when that is at or past floor, and
// computes the value again with compute in the background. A floor above that
// revision, such as a player's own write that dropped the value, is not
// answered: the reader opens a snapshot and waits on the change watch.
func (c *Cache) Recent(ctx context.Context, seqno, floor uint64, name string, compute func(context.Context, world.WorldState) (any, error)) (any, uint64, bool) {
	// Answer the value kept at seqno, marking it to be kept once dropped.
	// Only a finished value is marked, so a failed or unfinished compute is
	// never kept as a previous value.
	c.mtx.Lock()
	entry := c.entries[name]
	held := entry != nil && entry.at <= seqno && seqno <= c.seqno.Load()
	previous, dropped := c.previous[name]
	c.mtx.Unlock()
	if held {
		if value, ok := c.finishedValue(ctx, entry, seqno); ok {
			c.mtx.Lock()
			entry.recent = true
			c.mtx.Unlock()
			return value, seqno, true
		}
	}

	// Answer the dropped value at the newest revision it holds at.
	if !dropped || previous.through < floor || previous.entry.expired() {
		return nil, 0, false
	}
	c.refresh(name, compute)
	c.addViewReads(ctx, previous.entry)
	return previous.entry.value, previous.through, true
}

// refresh computes the named value again in the background at the World's
// Newest revision, at most once at a time per name, so the readers recent
// answers from the dropped value soon find a current one. It reads only, off
// the request path, and takes no write lock; nothing waits on it.
func (c *Cache) refresh(name string, compute func(context.Context, world.WorldState) (any, error)) {
	// Claim one background refresh per key.
	c.mtx.Lock()
	if _, ok := c.refreshing[name]; ok {
		c.mtx.Unlock()
		return
	}
	if c.refreshing == nil {
		c.refreshing = make(map[string]struct{})
	}
	c.refreshing[name] = struct{}{}
	c.mtx.Unlock()
	go func() {
		// Refresh under a bounded context and then release the claim.
		ctx, cancel := context.WithTimeout(context.Background(), backgroundReadTimeout)
		_, _ = c.getAtHead(ctx, name, compute)
		cancel()
		c.mtx.Lock()
		delete(c.refreshing, name)
		c.mtx.Unlock()
	}()
}

// PeekValue is c.peek for a value of type T.
func PeekValue[T any](ctx context.Context, c *Cache, seqno uint64, name string) (T, bool) {
	value, ok := c.Peek(ctx, seqno, name)
	if !ok {
		var zero T
		return zero, false
	}
	return value.(T), true
}
