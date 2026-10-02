package block

import (
	"context"
	"sync"
	"sync/atomic"
)

// Prefetch reads refs from store in order, with at most concurrency reads in
// flight, so a read-through store caches them before a program that reads
// them in the same order needs them. Reads start in list order; a read that
// fails does not stop the others. With WithReadAhead on ctx, a remote store
// widens each miss into a range read, so neighbouring blocks of one pack
// arrive together. Returns the first read error, or the context error when ctx
// ends first.
func Prefetch(ctx context.Context, store StoreOps, refs []*BlockRef, concurrency int) error {
	// Start one worker per allowed read, each claiming the next ref in order.
	var next atomic.Int64
	var errOnce sync.Once
	var firstErr error
	var wg sync.WaitGroup
	for range min(max(concurrency, 1), len(refs)) {
		wg.Go(func() {
			for ctx.Err() == nil {
				i := next.Add(1) - 1
				if i >= int64(len(refs)) {
					return
				}
				if _, _, err := store.GetBlock(ctx, refs[i]); err != nil {
					errOnce.Do(func() { firstErr = err })
				}
			}
		})
	}

	// Wait for the workers and report the first failure.
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	return firstErr
}
