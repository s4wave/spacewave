package s4db

import (
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

// lookupInFlight bounds the run lookups a batch keeps in flight, as the
// design's commit and GetBatch keep up to 32 reads outstanding.
const lookupInFlight = 32

// lookupSlow is the lookup time past which a batch starts more workers. A
// lookup that slow read the device, where overlapping reads pays; one served
// from the block cache or the page cache runs faster inline than a worker
// starts, and the scaling test measured workers doubling a hot batch's time.
const lookupSlow = 100 * time.Microsecond

// lookupEach looks up each of keys in st and calls fn with the key's index
// and its result. A key the memtable decides is answered at once. The rest
// go to the runs in order, inline until one lookup takes lookupSlow, and then
// on up to lookupInFlight workers, so cold reads overlap rather than queue.
// fn runs concurrently for distinct indices. With cache false the lookups
// read around the block cache, for a background pass, and an inline value fn
// receives lasts only until fn returns.
func (db *DB) lookupEach(st *state, keys [][]byte, cache bool, fn func(i int, v value, found bool) error) error {
	// Answer what the memtable holds, and queue the rest for the runs.
	var queued []int
	for i, k := range keys {
		e, ok := st.mem.get(k)
		if !ok {
			queued = append(queued, i)
			continue
		}
		if err := fn(i, e.val, e.val.kind != valueDeleted); err != nil {
			return err
		}
	}

	// work takes queued keys in order until none remain; the inline worker
	// starts the others once a lookup is slow.
	var next atomic.Int64
	var g errgroup.Group
	spawned := false
	var work func(inline bool) error
	work = func(inline bool) error {
		var sc *getScratch
		if !cache {
			sc = new(getScratch)
		}
		for {
			// Take the next key and look it up.
			n := int(next.Add(1) - 1)
			if n >= len(queued) {
				return nil
			}
			start := time.Now()
			i := queued[n]
			v, found, err := db.lookupRuns(st, keys[i], sc)
			if err != nil {
				return err
			}
			if err := fn(i, v, found); err != nil {
				return err
			}

			// Overlap the rest once a lookup read the device.
			if inline && !spawned && len(queued)-n > 2 && time.Since(start) >= lookupSlow {
				spawned = true
				for range min(lookupInFlight, len(queued)-n-1) - 1 {
					g.Go(func() error { return work(false) })
				}
			}
		}
	}

	// Run inline, then wait for any workers, which write fn's results, even
	// when the inline worker failed.
	err := work(true)
	if werr := g.Wait(); err == nil {
		err = werr
	}
	return err
}
