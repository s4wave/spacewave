// Package block_store_fillcache implements a best-effort disk cache of copies
// of blocks that a durable store holds, kept in a log of local packs.
//
// A read through the cache serves a block the cache holds, else reads the
// durable store and queues the block to be copied into the cache. Every failure
// of the cache reads as a miss, so a damaged, full, slow or missing cache costs
// only the read of the durable store. The directory may be deleted at any time
// and Open recreates it.
//
// The packs are a packfile/packcache Cache: fills append to the open pack
// without a sync, a hit is a lookup in a table in memory and one read of a
// pack, and eviction drops the oldest pack after carrying forward the blocks
// read since their write. A pack holds each block as the bucket's packs do, its
// bytes with its refs, under packfile.BlockKey of its hash.
package block_store_fillcache

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/s4wave/spacewave/db/packfile/packcache"
	"github.com/sirupsen/logrus"
)

// DefaultLimitBytes is the size bound of a cache whose Options set none.
const DefaultLimitBytes = 20 << 30

// DefaultQueueBytes bounds the block bytes awaiting their write when Options
// set no bound.
const DefaultQueueBytes = 128 << 20

// packsDir is the directory of the packs, inside the cache's directory.
const packsDir = "packs"

// Cache is a directory of packs of copies of blocks that a durable store
// holds.
//
// Invariant: the cache holds only copies of blocks that the durable store
// holds. Fill takes a block only from a read of that store, so a block that
// only another source holds, such as a read fallback, a peer or a writer's
// queue, never enters it. Losing the cache loses nothing.
//
// Reads never wait on the packs' writer: a fill only queues its block, and one
// goroutine appends the queue in batches. The packs load in the background
// after Open; until they have, reads miss and the queue holds the fills, past
// its bound dropping them.
type Cache struct {
	// le is the entry for the cache's log lines.
	le *logrus.Entry
	// warn limits the log lines about failures.
	warn rateLog
	// dir is the cache's directory: the packs and the resident lists.
	dir string
	// packs holds the copies.
	packs *packcache.Cache
	// view is the read-only view of the packs that View returns.
	view *view
	// cancel stops the writer.
	cancel context.CancelFunc
	// exited closes when the writer has returned.
	exited chan struct{}
	// bcast guards queue and wakes the writer and Wait.
	bcast broadcast.Broadcast
	// queue holds the fills awaiting their write.
	queue *fillQueue
	// warmBytes is the bytes WarmResident queues before it waits for the
	// writer: an eighth of the queue's bound.
	warmBytes int64
	// counts are the counters Counts reports.
	counts counters
}

// Open opens the cache in dir, creating it when missing; the packs load in
// the background, recovering what a crash left. The cache runs until Close or
// until ctx ends.
func Open(ctx context.Context, le *logrus.Entry, dir string, opts Options) (*Cache, error) {
	// Create the directory and clear the resident lists' killed saves.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, errors.Wrap(err, "create the block fill cache directory")
	}
	removeResidentTemps(dir)

	// Start loading the packs and the writer.
	limit, queueBytes := opts.LimitBytes, opts.QueueBytes
	if limit <= 0 {
		limit = DefaultLimitBytes
	}
	if queueBytes <= 0 {
		queueBytes = DefaultQueueBytes
	}
	cctx, cancel := context.WithCancel(ctx)
	c := &Cache{
		le:        le,
		warn:      rateLog{le: le},
		dir:       dir,
		packs:     packcache.Open(cctx, filepath.Join(dir, packsDir), packcache.Options{LimitBytes: limit}),
		cancel:    cancel,
		exited:    make(chan struct{}),
		queue:     newFillQueue(queueBytes),
		warmBytes: queueBytes / 8,
	}
	c.view = &view{cache: c}
	go c.run(cctx)
	return c, nil
}

// View returns the cache's read side as a block store: it serves the blocks
// the cache holds and nothing else.
func (c *Cache) View() block.StoreOps {
	return c.view
}

// Reads returns a source that reads the cache, then the durable store lower,
// and queues each of lower's hits for the cache. A nil lower yields a nil
// source, as no store holds the blocks to copy.
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
	c.enqueue(&block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: stored.RefsKnown})
	return nil
}

// remove forgets the copy of a block that failed its hash.
func (c *Cache) remove(ref *block.BlockRef) {
	c.packs.Remove(packfile.BlockKey(ref.GetHash()))
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

// Wait waits until no fill is queued or being written.
func (c *Cache) Wait(ctx context.Context) error {
	return c.bcast.Wait(ctx, func(func(), func() <-chan struct{}) (bool, error) {
		return c.queue.idle(), nil
	})
}

// Size returns the bytes of the cache's packs, zero before they load.
func (c *Cache) Size() int64 {
	return c.packs.Size()
}

// Close stops the writer, drops the fills still queued and closes the packs.
func (c *Cache) Close() {
	c.cancel()
	<-c.exited
	c.packs.Close()
}

// run appends the queued fills in batches until ctx ends, once the packs have
// loaded. A failed load leaves the packs unavailable, so each batch is
// dropped and counted.
func (c *Cache) run(ctx context.Context) {
	defer close(c.exited)
	if err := c.packs.Ready(ctx); err != nil && ctx.Err() == nil {
		c.le.WithError(err).Warn("the block fill cache could not load its packs, reads go to the durable store")
	}
	var blocks []packahead.Block
	for {
		// Take the next batch.
		var batch []*block.PutBatchEntry
		err := c.bcast.Wait(ctx, func(func(), func() <-chan struct{}) (bool, error) {
			batch = c.queue.take()
			return len(batch) != 0, nil
		})
		if err != nil {
			return
		}

		// Append it and release its blocks for queueing again.
		blocks = c.write(ctx, blocks[:0], batch)
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			c.queue.done(batch)
			broadcast()
		})
	}
}

// write appends batch to the packs, encoding each block with its refs into
// blocks, and returns blocks for reuse. A block it cannot encode, or a batch
// the packs refuse, is dropped and counted.
func (c *Cache) write(ctx context.Context, blocks []packahead.Block, batch []*block.PutBatchEntry) []packahead.Block {
	// Encode the blocks as the bucket's packs hold them.
	for _, entry := range batch {
		value, err := block.EncodeBlockObject(entry.Data, entry.Refs, entry.RefsKnown)
		if err != nil {
			c.counts.writeFailed.Add(1)
			c.warn.warn(err, "the block fill cache could not encode a block")
			continue
		}
		blocks = append(blocks, packahead.Block{Key: packfile.BlockKey(entry.Ref.GetHash()), Value: value})
	}

	// Append them, counting the result.
	err := c.packs.Put(ctx, blocks)
	switch {
	case err == nil:
		c.counts.written.Add(uint64(len(blocks)))
	case errors.Is(err, packcache.ErrUnavailable):
		c.counts.dropped.Add(uint64(len(blocks)))
	case ctx.Err() == nil:
		c.counts.writeFailed.Add(uint64(len(blocks)))
		c.warn.warn(err, "the block fill cache failed a write")
	}
	return blocks
}
