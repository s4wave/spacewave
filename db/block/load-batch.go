package block

import (
	"context"
	"time"
)

// BatchLoad gathers the blocks a walk reads next, such as one level of a tree,
// and reads those no cache holds with one batch read of the store. Add each
// block with AddRead, AddCursorRead, AddLeaf or AddCursorLeaf, then call Load,
// and read the blocks on the context it returns: each read, through
// UnmarshalReadBlock, a Cursor or a leaf fetch, takes the batch's answer once
// rather than read the store again, and keeps every effect of a store read: the
// cold-read admission, which the batch takes once, the counter, the access log,
// the hold, the decode and the decoded-block admission a durable store's mark
// allows. A block the batch failed to read fails its read with that error.
//
// The batch reads through the transaction and store of the first block added. A
// block of another transaction or store reads alone. Whether the cache holds a
// block is a guess made when it is added: a block it guessed cached reads
// alone, and an answer no read takes stays on the context unused. The context
// holds the answers until the reads take them, so the caller reads the blocks
// soon after, as a walk reads one level. The zero BatchLoad is empty.
type BatchLoad struct {
	// source is the position of the first block added, whose transaction and
	// store the batch reads through.
	source *Cursor
	// store is the store source reads through, or nil when it has none.
	store StoreOps
	// err is the failure to refresh the store's view of the cache.
	err error
	// refs are the blocks to read.
	refs []*BlockRef
}

// AddRead adds the block a read of T through r would fetch from the store.
func AddRead[T any, PT Ptr[T]](ctx context.Context, b *BatchLoad, r ReadCursor) {
	// Look the reader up in the cache its read would use, as
	// UnmarshalReadBlock does.
	if r.ref.GetEmpty() || !b.admit(ctx, r.source) {
		return
	}
	if !r.source.t.readOnly.Load() && decodedBlockCacheFromContext(ctx) == nil {
		ctx = WithDecodedBlockCache(ctx, r.source.t.readerCache())
	}
	if !wouldHit[T, PT](ctx, r) {
		b.refs = append(b.refs, r.ref)
	}
}

// AddCursorRead adds the block an Unmarshal of T at c would fetch from the
// store. A position that holds its decoded block, a sub-block, or a change of
// its own adds nothing.
func AddCursorRead[T any, PT Ptr[T]](ctx context.Context, b *BatchLoad, c *Cursor) {
	// A position reads through its transaction's read cache, as Unmarshal does.
	ref := c.unloadedRef()
	if ref == nil || !b.admit(ctx, c) {
		return
	}
	if !wouldHit[T, PT](ctx, ReadCursor{source: c, ref: ref}) {
		b.refs = append(b.refs, ref)
	}
}

// AddLeaf adds the leaf block a FetchLeaf through r reads. No cache holds a
// leaf, so it is always read.
func (b *BatchLoad) AddLeaf(ctx context.Context, r ReadCursor) {
	if !r.ref.GetEmpty() && b.admit(ctx, r.source) {
		b.refs = append(b.refs, r.ref)
	}
}

// AddCursorLeaf adds the leaf block a FetchLeaf at c reads, as AddLeaf does.
func (b *BatchLoad) AddCursorLeaf(ctx context.Context, c *Cursor) {
	if ref := c.unloadedRef(); ref != nil && b.admit(ctx, c) {
		b.refs = append(b.refs, ref)
	}
}

// Load reads the added blocks in one batch and returns ctx with the answers
// attached, or ctx itself when fewer than two blocks need a read.
func (b *BatchLoad) Load(ctx context.Context) (context.Context, error) {
	answers, err := b.Read(ctx)
	if err != nil {
		return ctx, err
	}
	return answers.Context(ctx), nil
}

// Read reads the added blocks in one batch and returns their answers, or nil
// when fewer than two blocks need a read: a single block reads as well alone.
func (b *BatchLoad) Read(ctx context.Context) (*BatchAnswers, error) {
	if b.err != nil || len(b.refs) < 2 {
		return nil, b.err
	}
	return readBatchRefs(ctx, b.store, b.refs)
}

// admit reports whether a block read at c joins the batch, taking c's
// transaction and store for the batch when it is the first.
func (b *BatchLoad) admit(ctx context.Context, c *Cursor) bool {
	// The first block's position sets the store, whose view of the cache is
	// refreshed before any block is looked up in it.
	if b.source == nil {
		if c == nil || c.t == nil {
			return false
		}
		b.source, b.store = c, c.readStore(ctx)
		if freshener, ok := b.store.(DecodedBlockCacheFreshener); ok {
			b.err = freshener.EnsureDecodedBlockCacheFresh(withoutReadOperationStore(ctx))
		}
	}
	return b.err == nil && b.store != nil && sameReadStore(c, b.source)
}

// sameReadStore reports whether c reads through the transaction and store of
// source, so one batch of source's store answers c's reads. A cursor with its
// own store override reads in the batch only when it is source itself.
func sameReadStore(c, source *Cursor) bool {
	return c != nil && c.t == source.t && (c == source || c.store == nil && source.store == nil)
}

// wouldHit reports whether a read of T through r would find its block in the
// decoded-block cache, as UnmarshalReadBlock and Cursor.Unmarshal look it up,
// without recording a hit or a miss.
func wouldHit[T any, PT Ptr[T]](ctx context.Context, r ReadCursor) bool {
	// The reader's own link holds a block it decoded before.
	if data := r.current(ctx); data != nil {
		if _, ok := data.block.(PT); ok {
			return true
		}
	}

	// Otherwise the read operation's front cache or the shared pool may
	// hold the block.
	c := r.loadCursor()
	key, ok := decodedBlockCacheKeyFor(r.ref, PT(new(T)), c.transformer())
	if !ok {
		return false
	}
	ctx = c.decodedBlockCacheContext(ctx)
	return decodedBlockCacheFromContext(ctx).holds(ctx, key)
}

// readBatchRefs reads refs from store in one batch and returns the answers.
func readBatchRefs(ctx context.Context, store StoreOps, refs []*BlockRef) (*BatchAnswers, error) {
	// Read the batch once the cold-read gate admits it, marking each block.
	if err := enterColdRead(ctx); err != nil {
		return nil, err
	}
	batchCtx, marks := withBlockReadMarks(withoutReadOperationStore(ctx), refs)
	answers := &BatchAnswers{answers: make(map[string]*batchAnswer, len(refs))}
	started := time.Now()
	err := GetStoredBlockBatch(batchCtx, store, refs, func(i int, stored *StoredBlock, err error) error {
		answers.answers[refs[i].MapKey()] = &batchAnswer{stored: stored, err: err, mark: marks[i]}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Share the batch's time among its blocks for the read counter.
	elapsed := time.Since(started) / time.Duration(len(refs))
	for _, a := range answers.answers {
		a.elapsed = elapsed
	}
	return answers, nil
}
