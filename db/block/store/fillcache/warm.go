package block_store_fillcache

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// ReadBatch reads the blocks of refs from the durable store with few
// requests, calls fn with each block it read and verified, and may skip any
// ref. Calls of fn never overlap. The pack store's ReadBatch is one.
type ReadBatch func(ctx context.Context, refs []*block.BlockRef, fn func(*block.BlockRef, *block.StoredBlock) error) error

// WarmCounts are the refs one WarmResident listed, found missing and fetched.
type WarmCounts struct {
	// Listed is the distinct refs the list named, as MissingResident counts.
	Listed int
	// Missing is the listed refs the cache lacked.
	Missing int
	// Fetched is the missing refs read and queued into the cache.
	Fetched int
}

// WarmResident reads the refs of scope's resident list that the cache lacks
// with read, and queues each block read into the cache, a slice of the list
// at a time. After every eighth of the queue's bound it waits for the writer,
// so the warm-up holds at most that much of the queue a foreground fill
// shares. It waits for the packs to load first, so it reads only what they
// lack. It stops at ctx's end, a failed load or the first failed read,
// returning what it did: the refs it left read on demand.
func (c *Cache) WarmResident(ctx context.Context, scope string, read ReadBatch) (WarmCounts, error) {
	// Wait for the packs, which say what the cache lacks.
	var counts WarmCounts
	if err := c.packs.Ready(ctx); err != nil {
		return counts, err
	}

	// Read each slice of missing refs, queueing each block it yields.
	var queued int64
	listed, err := c.MissingResident(ctx, scope, func(refs []*block.BlockRef) error {
		counts.Missing += len(refs)
		return read(ctx, refs, func(ref *block.BlockRef, stored *block.StoredBlock) error {
			// Queue the block, and let the writer catch up at each mark.
			c.enqueue(&block.PutBatchEntry{Ref: ref, Data: stored.Data, Refs: stored.Refs, RefsKnown: stored.RefsKnown})
			counts.Fetched++
			queued += int64(len(stored.Data))
			if queued < c.warmBytes {
				return nil
			}
			queued = 0
			return c.Wait(ctx)
		})
	})
	counts.Listed = listed
	return counts, err
}
