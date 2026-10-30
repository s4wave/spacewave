package kvtx

import (
	"context"
	"errors"
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// maxFillBatch bounds the cache fills one transaction writes, so a commit
// queued behind a fill batch waits for a bounded write.
const maxFillBatch = 128

// maxFillBatchBytes bounds the block bytes one fill transaction writes.
const maxFillBatchBytes = 8 << 20

// cacheFill is one queued cache fill and its result.
type cacheFill struct {
	owner string
	data  []byte
	opts  *block.PutOpts
	// ref, existed and err are set before done closes.
	ref     *block.BlockRef
	existed bool
	err     error
	done    chan struct{}
}

// fillQueue groups concurrent cache fills into shared transactions.
type fillQueue struct {
	mu sync.Mutex
	// pending are the fills no transaction has taken yet.
	pending []*cacheFill
	// draining records that a caller is writing the pending fills.
	draining bool
}

// prepareCacheFill writes a cache fill in a transaction shared with the other
// fills queued at the same time. A read through to another store fills every
// block it misses, and each fill alone would cost a commit and a turn of the
// store's writer, so a burst of misses after a restart would queue hundreds
// of commits ahead of the next head publication. Grouped, the burst costs a
// few commits, and a publication waits behind one bounded batch at most.
//
// The first fill queued with no drain running starts one, which writes batches
// until the queue is empty, so no caller works through the fills of others. A
// caller whose context ends stops waiting; its fill is still written.
func (v *Volume) prepareCacheFill(ctx context.Context, owner string, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Queue the fill, and drain the queue if no caller is.
	fill := &cacheFill{owner: owner, data: data, opts: opts, done: make(chan struct{})}
	q := &v.fills
	q.mu.Lock()
	q.pending = append(q.pending, fill)
	drain := !q.draining
	q.draining = true
	q.mu.Unlock()
	if drain {
		go v.drainCacheFills(context.WithoutCancel(ctx))
	}

	// Wait for the batch holding the fill.
	select {
	case <-fill.done:
		return fill.ref, fill.existed, fill.err
	case <-ctx.Done():
		return nil, false, context.Cause(ctx)
	}
}

// drainCacheFills writes the queued fills in bounded batches until none are
// left.
func (v *Volume) drainCacheFills(ctx context.Context) {
	q := &v.fills
	for {
		// Take the next batch, or end the drain on an empty queue.
		q.mu.Lock()
		if len(q.pending) == 0 {
			q.draining = false
			q.mu.Unlock()
			return
		}
		n, size := 0, 0
		for n < len(q.pending) && n < maxFillBatch {
			if n != 0 && size+len(q.pending[n].data) > maxFillBatchBytes {
				break
			}
			size += len(q.pending[n].data)
			n++
		}
		batch := q.pending[:n:n]
		q.pending = q.pending[n:]
		q.mu.Unlock()

		// Write each owner's fills in one transaction.
		for len(batch) != 0 {
			owner := batch[0].owner
			var group, rest []*cacheFill
			for _, fill := range batch {
				if fill.owner == owner {
					group = append(group, fill)
				} else {
					rest = append(rest, fill)
				}
			}
			v.writeCacheFills(ctx, owner, group)
			batch = rest
		}
	}
}

// writeCacheFills writes fills of one owner in one transaction and settles
// each. When the shared transaction fails, each fill is written alone, so one
// bad fill does not fail the others.
func (v *Volume) writeCacheFills(ctx context.Context, owner string, fills []*cacheFill) {
	err := v.prepareCacheFills(ctx, owner, fills)
	if err != nil && len(fills) > 1 && !errors.Is(err, block.ErrPublicationClosed) {
		for _, fill := range fills {
			v.writeCacheFills(ctx, owner, []*cacheFill{fill})
		}
		return
	}
	for _, fill := range fills {
		if err != nil {
			fill.ref, fill.existed, fill.err = nil, false, err
		}
		close(fill.done)
	}
}

// prepareCacheFills writes fills of one owner in one transaction and records
// each fill's ref and existence.
func (v *Volume) prepareCacheFills(ctx context.Context, owner string, fills []*cacheFill) error {
	// Write the fills.
	size := 0
	for _, fill := range fills {
		size += len(fill.data)
	}
	err := v.prepareOwned(ctx, owner, size, claimBucket, func(store block.StoreOps) error {
		for _, fill := range fills {
			ref, existed, err := store.PutBlock(ctx, fill.data, fill.opts)
			if err != nil {
				return err
			}
			fill.ref, fill.existed = ref, existed
		}
		return nil
	})

	// Count the blocks they stored.
	if err == nil {
		for _, fill := range fills {
			v.countPut(putFill, len(fill.data), fill.existed, fill.opts.GetRefsKnown())
		}
	}
	return err
}
