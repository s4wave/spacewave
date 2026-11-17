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
	"time"

	"github.com/aperturerobotics/util/broadcast"
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

// DefaultQueueBytes bounds the block bytes awaiting their write when Options
// set no bound.
const DefaultQueueBytes = 128 << 20

// Options configures a Cache.
type Options struct {
	// LimitBytes bounds the bytes of the cache's rows; past it, eviction
	// deletes rows down to 90% of it. Zero uses DefaultLimitBytes.
	LimitBytes int64
	// QueueBytes bounds the block bytes awaiting their write. A fill past it is
	// dropped, and the next read of the block queues it again. Zero uses
	// DefaultQueueBytes.
	QueueBytes int64
}

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
// The cache's reads are durable reads only over a durable store that keeps
// every block it holds, see Reads, so only their decoded blocks may enter the
// shared decoded-block pool. Every way a block leaves the file reports it
// removed before the removal commits and again after: the writer reports a
// tombstone for a block that failed its hash before its write, and the block
// store reports it after; an eviction deletes rows past the block store and
// reports each itself; opening the file, which may be new or replaced,
// reports every block removed, and recreating an unusable one reports them
// again after the delete. Close removes nothing: the file keeps its blocks.
type Cache struct {
	// le is the entry for the cache's log lines.
	le *logrus.Entry
	// warn limits the log lines about failures.
	warn rateLog
	// store is the SQLite database.
	store store_kvtx_sqlite.Store
	// blocks is the block store over the database.
	blocks *block_store_kvtx.KVTxBlock
	// view is the read-only view of blocks served by Reads over a durable
	// store that may remove blocks. Its reads are not durable.
	view *view
	// kept is the view served by Reads over a durable store that keeps every
	// block, whose reads are durable.
	kept *view
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
	// evicting is set while the cache is deleting rows down to its target.
	evicting bool
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

	// Build the cache over the database. Its block store is durable, so its
	// tombstones report their removals; only the kept view's reads mark
	// themselves durable.
	keys := store_kvkey.NewDefaultKVKey()
	c := &Cache{
		le:     le,
		warn:   rateLog{le: le},
		store:  store,
		blocks: block_store_kvtx.NewDurableKVTxBlock(keys, store, 0, true),
		exited: make(chan struct{}),
	}
	c.view = &view{StoreOps: block.NewReadOnlyStore(block_store_kvtx.NewKVTxBlock(keys, store, 0, true)), cache: c}
	c.kept = &view{StoreOps: block.NewReadOnlyStore(c.blocks), cache: c}

	// Bound the cache's rows and its queued fills.
	limit := opts.LimitBytes
	if limit <= 0 {
		limit = DefaultLimitBytes
	}
	c.evict = newEvicter(store.GetDB(), cacheTable, keys, limit)
	queueBytes := opts.QueueBytes
	if queueBytes <= 0 {
		queueBytes = DefaultQueueBytes
	}
	c.queue = newFillQueue(queueBytes)

	// Run the writer until Close.
	runCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.run(runCtx)
	return c, nil
}

// Reads returns a source that reads the cache, then the durable store lower,
// and queues each block of lower's hits for the cache, whether or not its refs
// are known: the cache keeps no refs and serves every block with its refs
// unknown. A nil lower yields a nil source, as no store holds the blocks to
// copy.
//
// The cache's reads are durable, so their decoded blocks may enter the shared
// decoded-block pool, only while lower declares with KeepsBlocks that it never
// removes a block it holds. A store that removes blocks, as reclaim does,
// cannot tell the cache, which would go on serving its copy; the pool then
// keeps nothing the cache serves.
func (c *Cache) Reads(lower block_store.StoreSource) block_store.StoreSource {
	return func() block.StoreOps {
		// Read nothing without a durable store to copy from.
		store := lower()
		if store == nil {
			return nil
		}

		// Read the kept view, whose reads are durable, only over a store that
		// keeps its blocks.
		view := c.view
		if keeper, ok := store.(blockKeeper); ok && keeper.KeepsBlocks() {
			view = c.kept
		}
		return block_store.NewCacheReadThrough(
			func() block.StoreOps { return view },
			func() block.StoreOps { return store },
			c.Fill,
		)
	}
}

// blockKeeper is a durable store that may declare it never removes a block it
// holds, as a pack store without reclaim does.
type blockKeeper interface {
	// KeepsBlocks reports that the store never removes a block it holds.
	KeepsBlocks() bool
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
			broadcast()
		}
	})
	if full {
		c.warn.warn(errors.New("the fill queue is full"), "the block fill cache dropped fills")
	}
}

// Wait waits until no fill is queued or being written and no eviction runs.
func (c *Cache) Wait(ctx context.Context) error {
	return c.bcast.Wait(ctx, func(func(), func() <-chan struct{}) (bool, error) {
		return c.queue.idle() && !c.evicting, nil
	})
}

// Size returns the bytes of the cache's rows.
func (c *Cache) Size(ctx context.Context) (int64, error) {
	return c.evict.used(ctx)
}

// Close stops the writer, drops the fills still queued and closes the file.
func (c *Cache) Close() error {
	c.cancel()
	<-c.exited
	return c.store.Close()
}

// run writes the queued entries in batches and evicts while the cache is over
// its bound, until ctx ends. It runs on its own goroutine from Open, so it
// starts even when ctx has ended, and Close always sees exited close.
func (c *Cache) run(ctx context.Context) {
	defer close(c.exited)

	// Mark the writes as background work a crash may lose, so a commit takes no
	// flush of its own.
	wctx := kvtx.WithBackgroundWriteContext(kvtx.WithOrderedCommitContext(ctx))
	for {
		// Wait for entries to write or an eviction to go on.
		var batch []*block.PutBatchEntry
		var evicting bool
		err := c.bcast.Wait(ctx, func(func(), func() <-chan struct{}) (bool, error) {
			batch, evicting = c.queue.take(), c.evicting
			return len(batch) != 0 || evicting, nil
		})
		if err != nil {
			return
		}

		// Write the entries, reporting their tombstones removed before the
		// write, as the block store reports them again after it.
		if len(batch) != 0 {
			block.InvalidateRemovedTombstones(batch)
			if _, err := c.blocks.PutBlockBatch(wctx, batch); err != nil && ctx.Err() == nil {
				c.warn.warn(err, "the block fill cache failed a write, dropping its fills")
			}
		}

		// Delete a run of rows while the cache is over its bound.
		evicting, err = c.evict.next(ctx, evicting)
		if err != nil && ctx.Err() == nil {
			c.warn.warn(err, "the block fill cache failed an eviction")
		}

		// Release the batch's blocks and publish the eviction state.
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			c.queue.done(batch)
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
