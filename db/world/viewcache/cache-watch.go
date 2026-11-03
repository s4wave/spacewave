package viewcache

import "context"

// changed returns a channel closed once anything recent would answer changes:
// the cache passes a newer revision, or a finished value replaces one a
// catch-up dropped. A value first computed at a revision the cache already
// stands at answers what a snapshot there reads, so it does not close it.
func (c *Cache) changed() <-chan struct{} {
	c.mtx.Lock()
	defer c.mtx.Unlock()
	if c.moved == nil {
		c.moved = make(chan struct{})
	}
	return c.moved
}

// movedLocked wakes the waiters on changed. The caller holds mtx.
func (c *Cache) movedLocked() {
	if c.moved != nil {
		close(c.moved)
		c.moved = nil
	}
}

// ChangeWait remembers the cache's progress channel before a caller reads.
// Wait then observes revisions and same-revision refreshes without polling.
type ChangeWait struct {
	// cache owns the revision and refresh notifications.
	cache *Cache
	// moved is the notification sampled before reading.
	moved <-chan struct{}
	// at is the revision sampled with this wait.
	at uint64
}

// NextChange samples progress before the caller reads a value.
func (c *Cache) NextChange() ChangeWait {
	return ChangeWait{cache: c, moved: c.changed(), at: c.seqno.Load()}
}

// Wait waits for the sampled progress or cancellation. An unwatched cache
// drives its own catch-up from the engine's next revision.
func (w ChangeWait) Wait(ctx context.Context) error {
	// Supply the next revision only when no shared change feed drives the cache.
	c := w.cache
	if c.watched.Load() == 0 {
		next, cancel := context.WithCancel(ctx)
		defer cancel()
		go func() {
			if seqno, err := c.engine.WaitSeqno(next, w.at+1); err == nil {
				c.catchUp(seqno)
			}
		}()
	}

	// Wait on the channel sampled before the read, including refresh progress.
	select {
	case <-w.moved:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
