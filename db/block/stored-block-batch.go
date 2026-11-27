package block

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// storedBlockEachReads is how many reads GetStoredBlockEach keeps in flight
// unless its context sets another, the number of child pages an Okra lookup
// loaded at once before batch reads.
const storedBlockEachReads = 16

// eachReadsContextKey is the context key of the GetStoredBlockEach width.
type eachReadsContextKey struct{}

// withEachReads returns ctx on which GetStoredBlockEach keeps at most n reads
// in flight, for a walk that sets how many blocks it reads at once, as a graph
// walk does.
func withEachReads(ctx context.Context, n int) context.Context {
	return context.WithValue(ctx, eachReadsContextKey{}, n)
}

// eachReads returns how many reads GetStoredBlockEach keeps in flight on ctx.
func eachReads(ctx context.Context) int {
	if n, ok := ctx.Value(eachReadsContextKey{}).(int); ok && n > 0 {
		return n
	}
	return storedBlockEachReads
}

// StoredBlockFunc receives the outcome of one ref of a batch read: index i of
// the batch's refs, its stored block or nil when the store lacks it, and err
// when reading that block failed. Returning an error ends the batch.
type StoredBlockFunc func(i int, stored *StoredBlock, err error) error

// StoredBlockBatcher is a store that reads many blocks with fewer requests
// than one each, such as a bucket that reads nearby blocks of a pack with one
// range read. A store that wraps another implements it to pass the batch down,
// applying to each block what its GetStoredBlock applies.
type StoredBlockBatcher interface {
	// GetStoredBlockBatch calls fn once for each index of refs, as
	// StoredBlockFunc describes. Calls of fn never overlap and follow no
	// promised order. A failed block read reaches fn as its err. An error fn
	// returns, or a failure of the whole batch, ends it.
	GetStoredBlockBatch(ctx context.Context, refs []*BlockRef, fn StoredBlockFunc) error
}

// GetStoredBlockBatch reads refs from store with its StoredBlockBatcher when
// it implements one, and with GetStoredBlockEach otherwise.
func GetStoredBlockBatch(ctx context.Context, store StoreOps, refs []*BlockRef, fn StoredBlockFunc) error {
	if len(refs) == 0 {
		return nil
	}
	if b, ok := store.(StoredBlockBatcher); ok {
		return b.GetStoredBlockBatch(ctx, refs, fn)
	}
	return GetStoredBlockEach(ctx, store, refs, fn)
}

// BatchBlockContext returns the context for a read of ref that a batch read
// on ctx makes alone, so what the read records about where its bytes came
// from, such as staged content, marks that block of the batch alone. A store
// that answers a batch with reads of single blocks passes each read this
// context.
func BatchBlockContext(ctx context.Context, ref *BlockRef) context.Context {
	// Narrow a batch's marks to the mark of ref.
	mark := blockReadMarkFromContext(ctx)
	if mark == nil || mark.batch == nil {
		return ctx
	}
	return context.WithValue(ctx, blockReadContextKey{}, blockReadMarkFor(ctx, ref))
}

// GetStoredBlockEach reads refs from store one GetStoredBlock at a time, at
// most storedBlockEachReads in flight or the number a graph walk set on ctx,
// and calls fn once for each index of
// refs. It is the batch read of a store without native batching. Calls of fn
// never overlap and follow no promised order. A failed block read reaches fn
// as its err and does not end the batch. An error fn returns, or the end of
// ctx, ends it, and the reads in flight are waited for.
func GetStoredBlockEach(ctx context.Context, store StoreOps, refs []*BlockRef, fn StoredBlockFunc) error {
	// Read each ref in turn, stopping early once the batch ends.
	var mtx sync.Mutex
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(eachReads(ctx))
	for i, ref := range refs {
		if egCtx.Err() != nil {
			break
		}
		eg.Go(func() error {
			stored, err := store.GetStoredBlock(BatchBlockContext(egCtx, ref), ref)
			mtx.Lock()
			defer mtx.Unlock()
			return fn(i, stored, err)
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	return context.Cause(ctx)
}

// GetStoredBlockBatchAt reads refs[at[j]] for each j from store as one batch
// with GetStoredBlockBatch, and calls fn with each block's index in refs. A
// wrapper that falls back to a second store reads in its second pass the
// indexes its first pass missed this way.
func GetStoredBlockBatchAt(ctx context.Context, store StoreOps, refs []*BlockRef, at []int, fn StoredBlockFunc) error {
	sub := make([]*BlockRef, len(at))
	for j, i := range at {
		sub[j] = refs[i]
	}
	return GetStoredBlockBatch(ctx, store, sub, func(j int, stored *StoredBlock, err error) error {
		return fn(at[j], stored, err)
	})
}
