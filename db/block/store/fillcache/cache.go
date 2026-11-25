// Package block_store_fillcache implements a best-effort block cache in its own
// SQLite file, for copies of blocks that a durable store holds.
//
// A read through the cache serves a block the cache holds, else reads the
// durable store and queues the block to be copied into the cache. Every failure
// of the cache reads as a miss, so a damaged, full, slow or missing cache costs
// only the read of the durable store. The file may be deleted at any time and
// Open recreates it.
package block_store_fillcache

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_sqlite "github.com/s4wave/spacewave/db/store/kvtx/sqlite"
	"github.com/sirupsen/logrus"
)

// DefaultLimitBytes is the size bound of a cache whose Options set none.
const DefaultLimitBytes = 20 << 30

// DefaultStartWindow is the start window of a cache whose Options set none.
const DefaultStartWindow = 10 * time.Minute

// DefaultQueueBytes bounds the block bytes awaiting their write when Options
// set no bound.
const DefaultQueueBytes = 128 << 20

// Cache is a SQLite file of copies of blocks that a durable store holds.
//
// Invariant: the cache holds only copies of blocks that the durable store
// holds. Fill takes a block only from a read of that store, so a block that
// only another source holds, such as a read fallback, a peer or a writer's
// queue, never enters it. Losing the cache loses nothing.
//
// One goroutine writes the file, in batched transactions, so fills never queue
// behind each other for SQLite's write lock and a read never waits for a write.
//
// The start window opens at Open. While it is open, eviction spares the range
// the previous start stored. When it closes, the writer replaces that range
// with the sequences this start stamped, from the first of Open to the last
// stamped by then. A process that exits inside its window, or stamps nothing
// in it, leaves the previous range in place, so the next start still finds the
// last completed start's blocks.
type Cache struct {
	// le is the entry for the cache's log lines.
	le *logrus.Entry
	// warn limits the log lines about failures.
	warn rateLog
	// path is the database file; resident lists sit beside it.
	path string
	// store is the SQLite database.
	store store_kvtx_sqlite.Store
	// blocks is the block store over the database.
	blocks *block_store_kvtx.KVTxBlock
	// view is the read-only view of blocks served by Reads.
	view *view
	// evict deletes rows when the cache passes its size bound.
	evict *evicter
	// cancel stops the writer.
	cancel context.CancelFunc
	// exited closes when the writer has returned.
	exited chan struct{}
	// bcast guards queue and evicting, and wakes the writer and Wait.
	bcast broadcast.Broadcast
	// queue holds the entries awaiting their write.
	queue *fillQueue
	// warmBytes is the bytes WarmResident queues before it waits for the
	// writer: an eighth of the queue's bound.
	warmBytes int64
	// evicting is set while the cache is deleting rows down to its target.
	evicting bool
	// reads holds the table keys of the hits awaiting their read stamp.
	reads map[string]struct{}
	// stamping is set while the writer stamps hits it took from reads.
	stamping bool
	// seq is the read sequence of the next stamp; only the writer uses it.
	seq int64
	// startSeq is the first read sequence of this start's window.
	startSeq int64
	// window closes the start window when it fires.
	window *time.Timer
	// windowOnce closes the start window once.
	windowOnce sync.Once
	// windowDue is set, under bcast, when the start window has closed and the
	// writer has yet to store its range.
	windowDue bool
	// windowDone closes when the writer has handled the closed start window.
	windowDone chan struct{}
	// counts are the counters Counts reports.
	counts counters
}

// Open opens the cache at path, creating it when missing and recreating it when
// the file is unusable. The cache runs until Close or until ctx ends.
func Open(ctx context.Context, le *logrus.Entry, path string, opts Options) (*Cache, error) {
	// Create the directory of the file.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, errors.Wrap(err, "create the block fill cache directory")
	}

	// Open the database, recreating an unusable file.
	store, err := openStore(ctx, le, path)
	if err != nil {
		return nil, errors.Wrap(err, "open the block fill cache")
	}
	removeResidentTemps(path)

	// Build the cache over the database.
	keys := store_kvkey.NewDefaultKVKey()
	c := &Cache{
		le:         le,
		path:       path,
		warn:       rateLog{le: le},
		store:      store,
		blocks:     block_store_kvtx.NewKVTxBlock(keys, store, 0),
		exited:     make(chan struct{}),
		reads:      make(map[string]struct{}),
		windowDone: make(chan struct{}),
	}
	c.view = &view{StoreOps: block.NewReadOnlyStore(c.blocks), cache: c}

	// Bound the cache, counting the rows eviction deletes, taking the default
	// for an unset size.
	limit := opts.LimitBytes
	if limit <= 0 {
		limit = DefaultLimitBytes
	}
	c.evict = newEvicter(store.GetDB(), cacheTable, keys, limit, &c.counts.evicted)

	// Continue the read sequence the file holds.
	if err := createReads(ctx, store.GetDB()); err != nil {
		_ = store.Close()
		return nil, err
	}
	last, err := lastReadSeq(ctx, store.GetDB())
	if err != nil {
		_ = store.Close()
		return nil, err
	}
	c.seq = last + 1
	c.startSeq = c.seq

	// Spare the previous start's range until this start's window closes.
	if lo, hi, ok, err := startRange(ctx, store.GetDB()); err != nil {
		_ = store.Close()
		return nil, err
	} else if ok {
		c.evict.protect(lo, hi)
	}

	// Bound the fills awaiting their write.
	queueBytes := opts.QueueBytes
	if queueBytes <= 0 {
		queueBytes = DefaultQueueBytes
	}
	c.queue = newFillQueue(queueBytes)
	c.warmBytes = queueBytes / 8

	// Run the writer until Close.
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	rc := routine.NewRoutineContainer()
	rc.SetRoutine(c.run)
	rc.SetContext(runCtx, false)

	// Close the start window when it ends.
	window := opts.StartWindow
	if window <= 0 {
		window = DefaultStartWindow
	}
	c.window = time.AfterFunc(window, c.closeWindow)
	return c, nil
}

// closeWindow marks the start window closed and wakes the writer to store its
// range.
func (c *Cache) closeWindow() {
	c.windowOnce.Do(func() {
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			c.windowDue = true
			broadcast()
		})
	})
}

// storeWindow replaces the protected range with this start's, the sequences
// from startSeq through the last stamped, and spares it from now on. A window
// that stamped nothing keeps the previous range. Only the writer calls it.
func (c *Cache) storeWindow(ctx context.Context) error {
	// Take this start's range, keeping the previous one when it is empty.
	defer close(c.windowDone)
	lo, hi := c.startSeq, c.seq-1
	if hi < lo {
		return nil
	}

	// Store the range, then spare it.
	if err := storeStartRange(ctx, c.store.GetDB(), lo, hi); err != nil {
		return err
	}
	c.evict.protect(lo, hi)
	return nil
}

// Reads returns a source that reads the cache, then the durable store lower,
// and queues each block of lower's hits that carries its refs for the cache. A
// nil lower yields a nil source, as no store holds the blocks to copy.
func (c *Cache) Reads(lower block_store.StoreSource) block_store.StoreSource {
	return func() block.StoreOps {
		store := lower()
		if store == nil {
			return nil
		}
		return block_store.NewStoreReadThrough(
			func() block.StoreOps { return c.view },
			func() block.StoreOps { return store },
			c.Fill,
		)
	}
}

// Fill queues a block read from the durable store to be copied into the cache.
// It returns before the write and drops the block when the queue is full. It
// matches block_store.Fill and ignores the store it is given.
func (c *Cache) Fill(_ context.Context, _ block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error {
	c.enqueue(&block.PutBatchEntry{Ref: ref, Data: stored.Data})
	return nil
}

// remove queues the removal of a block that failed its hash.
func (c *Cache) remove(ref *block.BlockRef) {
	c.enqueue(&block.PutBatchEntry{Ref: ref, Tombstone: true})
}

// enqueue queues the entry for the writer.
func (c *Cache) enqueue(entry *block.PutBatchEntry) {
	var full bool
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		var queued bool
		if queued, full = c.queue.push(entry); queued {
			c.counts.queued.Add(1)
			broadcast()
		}
	})
	if full {
		c.counts.dropped.Add(1)
		c.warn.warn(errors.New("the fill queue is full"), "the block fill cache dropped fills")
	}
}

// Wait waits until no fill is queued or being written, no hit awaits its read
// stamp, no closed start window awaits its range and no eviction runs.
func (c *Cache) Wait(ctx context.Context) error {
	return c.bcast.Wait(ctx, func(func(), func() <-chan struct{}) (bool, error) {
		return c.queue.idle() && len(c.reads) == 0 && !c.stamping && !c.windowDue && !c.evicting, nil
	})
}

// Size returns the bytes of the cache's rows.
func (c *Cache) Size(ctx context.Context) (int64, error) {
	return c.evict.used(ctx)
}

// Close stops the writer, drops the fills still queued and closes the file.
func (c *Cache) Close() error {
	c.window.Stop()
	c.cancel()
	<-c.exited
	return c.store.Close()
}

// run writes the queued entries in batches, stamps the rows they wrote and the
// rows hits read, stores the start range when the window closes, and evicts
// while the cache is over its bound, until ctx ends.
func (c *Cache) run(ctx context.Context) error {
	defer close(c.exited)

	// Mark the writes as background work a crash may lose, so a commit takes no
	// flush of its own.
	wctx := block.WithBackground(kvtx.WithOrderedCommitContext(ctx))
	for {
		// Wait for entries to write, hits to stamp or an eviction to go on.
		var batch []*block.PutBatchEntry
		var hits map[string]struct{}
		var evicting, windowDue bool
		err := c.bcast.Wait(ctx, func(func(), func() <-chan struct{}) (bool, error) {
			batch, evicting, windowDue = c.queue.take(), c.evicting, c.windowDue
			if len(c.reads) != 0 {
				hits, c.reads = c.reads, make(map[string]struct{})
				c.stamping = true
			}
			return len(batch) != 0 || len(hits) != 0 || evicting || windowDue, nil
		})
		if err != nil {
			return nil
		}

		// Write the entries.
		written := batch
		if len(batch) != 0 {
			if _, err := c.blocks.PutBlockBatch(wctx, batch); err != nil {
				written = nil
				c.counts.writeFailed.Add(uint64(len(batch)))
				if ctx.Err() == nil {
					c.warn.warn(err, "the block fill cache failed a write, dropping its fills")
				}
			} else {
				c.counts.written.Add(uint64(len(batch)))
			}
		}

		// Stamp the written rows and the hits with the next read sequence. A
		// failed stamp leaves rows looking older than they are.
		if len(written) != 0 || len(hits) != 0 {
			if err := c.stamp(ctx, c.seq, written, hits); err != nil && ctx.Err() == nil {
				c.warn.warn(err, "the block fill cache failed to stamp its reads")
			}
			c.seq++
		}

		// Store the start range once the window has closed, after the stamps
		// of the hits taken with it.
		if windowDue {
			if err := c.storeWindow(ctx); err != nil && ctx.Err() == nil {
				c.warn.warn(err, "the block fill cache failed to store its start range")
			}
		}

		// Delete a run of rows while the cache is over its bound.
		evicting, err = c.evict.next(ctx, evicting)
		if err != nil && ctx.Err() == nil {
			c.warn.warn(err, "the block fill cache failed an eviction")
		}

		// Release the batch's blocks and publish the eviction state.
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// Clear what this turn handled, keeping a window that closed
			// since the turn began, and wake the waiters.
			c.queue.done(batch)
			c.stamping = false
			c.windowDue = c.windowDue && !windowDue
			c.evicting = evicting
			broadcast()
		})

		// Pace eviction below the disk's speed when no fill waits.
		if evicting {
			c.pause(ctx)
		}
	}
}

// pause waits out evictPause, or less when a fill is queued meanwhile.
func (c *Cache) pause(ctx context.Context) {
	// Subscribe to the next queued fill.
	var queued <-chan struct{}
	c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		queued = getWaitCh()
	})

	// Wait for the pause or the fill.
	timer := time.NewTimer(evictPause)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	case <-queued:
	}
}
