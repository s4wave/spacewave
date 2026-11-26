package block

import (
	"context"
	"slices"

	"github.com/pkg/errors"
)

// graphBatchReads is how many batch reads a graph walk keeps in flight. A
// batch frees its slot only once its last block lands, so a few keep the walk
// reading while one batch waits on its slowest block.
const graphBatchReads = 4

// graphReader reads the queued blocks of a graph walk in batches: at most
// graphBatchReads batches in flight, each of at most size blocks, so the walk
// reads as many blocks at once as it did one block per read, and holds no more
// of them. A store that answers a batch one block at a time reads every block
// of it at once.
//
// Only the walk's goroutine calls fill, next and close.
type graphReader[N any] struct {
	// src is the store the walk reads.
	src StoreOps
	// ref returns the block ref of a queued node.
	ref func(N) *BlockRef
	// size is the most blocks one batch reads.
	size int
	// results receives each block read and the end of each batch. Its buffer
	// holds every send of the batches in flight, so a batch never waits on
	// the walk.
	results chan graphRead[N]
	// batches counts the batches in flight.
	batches int
}

// graphRead is one block a graphReader read, or the end of a batch.
type graphRead[N any] struct {
	// node is the queued node the block was read for.
	node N
	// stored is the block with its refs.
	stored *StoredBlock
	// err is the block's read error, wrapped with its ref, or the batch's
	// failure at its end.
	err error
	// end marks the end of a batch.
	end bool
}

// newGraphReader returns a reader of src keeping reads blocks in flight.
// Zero reads selects DefaultGraphCopyReads.
func newGraphReader[N any](src StoreOps, reads int, ref func(N) *BlockRef) *graphReader[N] {
	if reads <= 0 {
		reads = DefaultGraphCopyReads
	}
	size := max(1, reads/graphBatchReads)
	return &graphReader[N]{
		src:     src,
		ref:     ref,
		size:    size,
		results: make(chan graphRead[N], graphBatchReads*(size+1)),
	}
}

// fill starts a batch read of the nodes at the end of queue while a slot is
// free, so the walk keeps its depth-first order, and returns what remains.
func (r *graphReader[N]) fill(ctx context.Context, queue []N) []N {
	for r.batches < graphBatchReads && len(queue) != 0 {
		n := min(r.size, len(queue))
		batch := slices.Clone(queue[len(queue)-n:])
		queue = queue[:len(queue)-n]
		r.batches++
		go r.read(ctx, batch)
	}
	return queue
}

// read reads batch from src and sends each block, then the batch's end.
func (r *graphReader[N]) read(ctx context.Context, batch []N) {
	refs := make([]*BlockRef, len(batch))
	for i, node := range batch {
		refs[i] = r.ref(node)
	}
	err := GetStoredBlockBatch(withEachReads(ctx, len(refs)), r.src, refs, func(i int, stored *StoredBlock, err error) error {
		r.results <- graphRead[N]{node: batch[i], stored: stored, err: graphBlockErr(refs[i], stored, err)}
		return nil
	})
	r.results <- graphRead[N]{err: err, end: true}
}

// next returns the next block read or batch end, or false when no batch is in
// flight.
func (r *graphReader[N]) next() (graphRead[N], bool) {
	if r.batches == 0 {
		return graphRead[N]{}, false
	}
	res := <-r.results
	if res.end {
		r.batches--
	}
	return res, true
}

// close waits for the batches in flight, whose context the walk ended.
func (r *graphReader[N]) close() {
	for r.batches != 0 {
		r.next()
	}
}

// graphBlockErr returns the error of a walk's read of ref: the read's err, or
// ErrNotFound for a missing block and ErrRefsUnknown for a block held without
// its refs, wrapped with the block ref.
func graphBlockErr(ref *BlockRef, stored *StoredBlock, err error) error {
	if err == nil && stored == nil {
		err = NotFoundError(ref)
	}
	if err == nil && !stored.RefsKnown {
		err = ErrRefsUnknown
	}

	// Name the ref only on failure: encoding it costs more than the read.
	if err == nil {
		return nil
	}
	return errors.Wrap(err, ref.MarshalString())
}
