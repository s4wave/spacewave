package block

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/conc"
)

// Prefetch reads refs from store in order, with at most concurrency reads in
// flight, so a read-through store caches them before a program that reads
// them in the same order needs them. Reads start in list order; a read that
// fails does not stop the others. With WithReadAhead on ctx, a remote store
// widens each miss into a range read, so neighbouring blocks of one pack
// arrive together. Returns the first read error, or the context error when ctx
// ends first.
func Prefetch(ctx context.Context, store StoreOps, refs []*BlockRef, concurrency int) error {
	// Queue one read per ref. The queue starts them in list order with at
	// most concurrency in flight, and a read queued after ctx ends is skipped.
	var errOnce sync.Once
	var firstErr error
	reads := conc.NewConcurrentQueue(max(concurrency, 1))
	for _, ref := range refs {
		reads.Enqueue(func() {
			if ctx.Err() != nil {
				return
			}
			if _, _, err := store.GetBlock(ctx, ref); err != nil {
				errOnce.Do(func() { firstErr = err })
			}
		})
	}

	// Wait for the reads in flight and report the first failure.
	_ = reads.WaitIdle(context.Background(), nil)
	if err := ctx.Err(); err != nil {
		return err
	}
	return firstErr
}
