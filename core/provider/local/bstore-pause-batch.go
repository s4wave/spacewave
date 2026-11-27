package provider_local

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/db/block"
)

// pausedBatchAlternates is the number of alternate reads a paused batch keeps
// in flight, as GetStoredBlockEach bounds a store's reads.
const pausedBatchAlternates = 16

// GetStoredBlockBatch reads refs through the chain in one batch, racing the
// alternates for each block while the backend is paused, as raceAlternates
// races them for one read. Once the backend pauses, each block the batch has
// not answered asks the alternates in order, each once and only while the
// backend is paused. An alternate's hit answers the block and the batch's later
// answer for it is dropped. Once every block is answered the batch is
// cancelled. Calls of fn never overlap.
func (r *pausedReads) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	b := &pausedBatch{r: r, refs: refs, fn: fn, answered: make([]bool, len(refs)), left: len(refs)}
	if b.left == 0 {
		return nil
	}
	b.done = make(chan struct{})
	return b.run(ctx)
}

// pausedBatch is one batch read of pausedReads.
type pausedBatch struct {
	// r is the paused reads.
	r *pausedReads
	// refs are the blocks.
	refs []*block.BlockRef
	// fn takes each answer.
	fn block.StoredBlockFunc

	// mtx guards the fields below and serializes calls of fn.
	mtx sync.Mutex
	// answered marks each block answered.
	answered []bool
	// left counts the blocks not yet answered.
	left int
	// err is the first error fn returned.
	err error
	// done is closed once every block is answered or fn failed.
	done chan struct{}
}

// run reads the batch, starting the races at the first pause.
func (b *pausedBatch) run(ctx context.Context) error {
	// Read the chain beside the pause watch.
	ctx, cancel := context.WithCancel(ctx)
	var races sync.WaitGroup
	defer func() {
		cancel()
		races.Wait()
	}()
	batchDone := make(chan error, 1)
	go func() {
		batchDone <- block.GetStoredBlockBatch(ctx, b.r.StoreOps, b.refs, b.answer)
	}()

	// Race the unanswered blocks once the backend pauses.
	raced := false
	for {
		var changed <-chan struct{}
		if !raced {
			var paused bool
			paused, changed = b.r.pause()
			if paused {
				raced, changed = true, nil
				b.race(ctx, &races)
			}
		}
		select {
		case err := <-batchDone:
			if ferr := b.failed(); ferr != nil {
				return ferr
			}
			return err
		case <-b.done:
			cancel()
			<-batchDone
			return b.failed()
		case <-changed:
		}
	}
}

// race starts a racer for each block not yet answered, under races.
func (b *pausedBatch) race(ctx context.Context, races *sync.WaitGroup) {
	slots := make(chan struct{}, pausedBatchAlternates)
	b.mtx.Lock()
	defer b.mtx.Unlock()
	for i, done := range b.answered {
		if !done {
			races.Go(func() { b.raceOne(ctx, i, slots) })
		}
	}
}

// raceOne asks the alternates for block i in order, each once the backend is
// paused, until one hits, the block is answered, or ctx ends. slots bounds the
// alternate reads in flight.
func (b *pausedBatch) raceOne(ctx context.Context, i int, slots chan struct{}) {
	ref := b.refs[i]
	for _, alt := range b.r.alternates {
		// Wait for a pause, leaving once the block is answered.
		for {
			if b.isAnswered(i) {
				return
			}
			paused, changed := b.r.pause()
			if paused {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-changed:
			}
		}

		// Ask the alternate, serving a hit.
		select {
		case <-ctx.Done():
			return
		case slots <- struct{}{}:
		}
		stored, err := alt(block.BatchBlockContext(ctx, ref), ref)
		<-slots
		if err == nil && stored != nil {
			_ = b.answer(i, stored, nil)
			return
		}
	}
}

// failed returns the first error fn returned, or nil.
func (b *pausedBatch) failed() error {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.err
}

// isAnswered reports whether block i is answered or the batch has failed.
func (b *pausedBatch) isAnswered(i int) bool {
	b.mtx.Lock()
	defer b.mtx.Unlock()
	return b.answered[i] || b.err != nil
}

// answer passes the first answer for block i to fn and drops the rest.
func (b *pausedBatch) answer(i int, stored *block.StoredBlock, err error) error {
	// Drop the answers after a block's first answer and after a failure.
	b.mtx.Lock()
	defer b.mtx.Unlock()
	if b.err != nil {
		return b.err
	}
	if b.answered[i] {
		return nil
	}

	// Pass on the first answer, closing done once every block is answered
	// or fn fails.
	b.answered[i] = true
	b.left--
	if ferr := b.fn(i, stored, err); ferr != nil {
		b.err = ferr
		close(b.done)
		return ferr
	}
	if b.left == 0 {
		close(b.done)
	}
	return nil
}

// _ is a type assertion
var _ block.StoredBlockBatcher = (*pausedReads)(nil)
