package block

import (
	"context"
	"sync"

	"golang.org/x/sync/errgroup"
)

// storedBlockEachReads is how many reads GetStoredBlockEach keeps in flight,
// the number of child pages an Okra lookup loaded at once before batch reads.
const storedBlockEachReads = 16

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
	// promised order. A failed block read reaches fn as its err; an error fn
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

// GetStoredBlockEach reads refs from store one GetStoredBlock at a time, at
// most storedBlockEachReads in flight, and calls fn once for each index of
// refs. It is the batch read of a store without native batching. Calls of fn
// never overlap and follow no promised order. A failed block read reaches fn
// as its err and does not end the batch; an error fn returns, or the end of
// ctx, ends it, and the reads in flight are waited for.
func GetStoredBlockEach(ctx context.Context, store StoreOps, refs []*BlockRef, fn StoredBlockFunc) error {
	// Read each ref in turn, stopping early once the batch ends.
	var mtx sync.Mutex
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(storedBlockEachReads)
	for i, ref := range refs {
		if egCtx.Err() != nil {
			break
		}
		eg.Go(func() error {
			stored, err := store.GetStoredBlock(egCtx, ref)
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
