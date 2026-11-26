// Package packcache keeps copies of values in a packahead Log of local packs,
// bounded in size, as a disk cache in front of a slower store.
//
// The design follows the large object cache of CacheLib's Navy: a log of
// large regions with an index in memory, evicted a whole region at a time,
// oldest first, after putting back the items read since their write. Here a
// region is a pack, the index is the Log's table, and a fill is an append
// that does not wait for a sync, since a lost copy costs only a read.
//
// # Invariants
//
//   - A value Get returns is one some Put took under that key, or bytes a
//     pack's batch checksums matched at recovery: a torn batch is a miss, not
//     wrong data. The caller still checks a value against its key's hash.
//   - Losing the cache loses only copies, so nothing here syncs for a reader,
//     and every finalized pack counts as uploaded: the store below holds it.
//   - Eviction takes the oldest finalized pack once the Log passes the limit,
//     carrying forward the values read since their write, at most half the
//     pack's bytes, so each eviction frees at least half a pack.
//   - Until the Log has loaded, Get misses and Put stores nothing, so opening
//     the cache never delays its caller.
package packcache

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/packfile/packahead"
)

// ErrUnavailable is Put's result while the cache holds no loaded Log: before
// the load ends, after a failure and after Close.
var ErrUnavailable = errors.New("packcache: the cache is not loaded")

// Cache is a bounded disk cache of values under pack index keys.
type Cache struct {
	// opts are the cache's options.
	opts Options
	// log is the loaded Log, nil until the load ends.
	log atomic.Pointer[packahead.Log]
	// bcast guards state and wakes Ready's waiters.
	bcast broadcast.Broadcast
	// state is the cache's place in its life.
	state state
	// err is the load's or the evictor's failure.
	err error
	// cancel ends the load and the evictor.
	cancel context.CancelFunc
	// done closes when the load and the evictor have ended and the Log is
	// closed.
	done chan struct{}
	// hits, misses, evicted and carried back Counts.
	hits, misses, evicted, carried atomic.Uint64
}

// state is a Cache's place in its life.
type state int

const (
	// stateLoading opens the Log; Get misses and Put stores nothing.
	stateLoading state = iota
	// stateReady serves from the Log.
	stateReady
	// stateFailed holds a load or eviction failure; the Log is closed and
	// Get misses.
	stateFailed
	// stateClosed has closed the Log at Close.
	stateClosed
)

// Open starts loading the cache in dir and returns at once. ctx bounds the
// load and the evictor, which also end at Close.
func Open(ctx context.Context, dir string, opts Options) *Cache {
	cctx, cancel := context.WithCancel(ctx)
	c := &Cache{opts: opts, cancel: cancel, done: make(chan struct{})}
	go c.run(cctx, dir)
	return c
}

// run loads the Log and evicts until ctx ends or an eviction fails, then
// closes the Log.
func (c *Cache) run(ctx context.Context, dir string) {
	// Load the Log and serve from it.
	defer close(c.done)
	l, err := packahead.Open(ctx, dir, c.opts.Log)
	if err != nil {
		c.settle(stateFailed, err)
		return
	}
	c.log.Store(l)
	c.settle(stateReady, nil)

	// Restore reads from before the restart so the first eviction sees them.
	if c.opts.Recent != nil {
		c.opts.Recent(ctx, func(keys [][]byte) { l.MarkRead(keys) })
	}

	// Evict until the end, then stop serving and close the Log.
	err = c.evict(ctx, l)
	c.log.Store(nil)
	l.Close()
	if err != nil && ctx.Err() == nil {
		c.settle(stateFailed, err)
	}
}

// settle moves a loading or ready cache to next, recording err.
func (c *Cache) settle(next state, err error) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if c.state != stateLoading && c.state != stateReady {
			return
		}
		c.state, c.err = next, err
		broadcast()
	})
}

// evict marks each finalized pack uploaded, as the store below the cache
// holds every value it copies, so a removal applies at once. It takes the
// oldest finalized pack while l passes the limit, carrying its read values
// forward, and waits for l to change otherwise.
func (c *Cache) evict(ctx context.Context, l *packahead.Log) error {
	for {
		packs, changed := l.GetPacks()
		for _, p := range packs {
			if !p.Uploaded {
				l.MarkUploaded(p.Seq)
			}
		}
		if c.opts.LimitBytes > 0 && len(packs) != 0 && l.GetStats().LocalBytes > c.opts.LimitBytes {
			n, err := l.EvictCarry(ctx, packs[0].Seq, packs[0].Size/2)
			if err != nil {
				return err
			}
			c.evicted.Add(1)
			c.carried.Add(uint64(n)) //nolint:gosec // non-negative.
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-changed:
		}
	}
}

// Ready returns once the cache has loaded, failed or closed, with the
// failure, or ctx's cause.
func (c *Cache) Ready(ctx context.Context) error {
	for {
		var s state
		var err error
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			s, err, waitCh = c.state, c.err, getWaitCh()
		})
		if s != stateLoading {
			return err
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// Get returns a copy of the value under key and whether the cache holds it.
// A read marks the value to be carried forward at its pack's eviction.
func (c *Cache) Get(key []byte) ([]byte, bool, error) {
	// Miss until the Log has loaded.
	l := c.log.Load()
	if l == nil {
		c.misses.Add(1)
		return nil, false, nil
	}

	// Read the Log and count the result.
	value, found, err := l.Get(key)
	if found {
		c.hits.Add(1)
	} else {
		c.misses.Add(1)
	}
	return value, found, err
}

// Put stores copies of blocks, returning once they are written, without a
// sync. Before the load, and after a failure or Close, it stores nothing and
// returns ErrUnavailable.
func (c *Cache) Put(ctx context.Context, blocks []packahead.Block) error {
	l := c.log.Load()
	if l == nil {
		return ErrUnavailable
	}
	return l.AppendUnsynced(ctx, blocks)
}

// Has reports whether the cache holds a value under key, false before the
// load.
func (c *Cache) Has(key []byte) bool {
	l := c.log.Load()
	return l != nil && l.Has(key)
}

// Remove forgets the copy under key, such as one that failed its hash check.
func (c *Cache) Remove(key []byte) {
	if l := c.log.Load(); l != nil {
		l.Remove(key)
	}
}

// Size returns the local packs' total size, zero before the load.
func (c *Cache) Size() int64 {
	if l := c.log.Load(); l != nil {
		return l.GetStats().LocalBytes
	}
	return 0
}

// Counts returns the cache's running totals.
func (c *Cache) Counts() Counts {
	return Counts{
		Hits:    c.hits.Load(),
		Misses:  c.misses.Load(),
		Evicted: c.evicted.Load(),
		Carried: c.carried.Load(),
	}
}

// Close ends the load and the evictor and closes the Log, returning once it
// is closed.
func (c *Cache) Close() {
	c.cancel()
	<-c.done
	c.settle(stateClosed, nil)
}
