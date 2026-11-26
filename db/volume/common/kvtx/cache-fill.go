package kvtx

import (
	"context"
	"errors"
	"slices"
	"sync"

	"github.com/aperturerobotics/util/ulid"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
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
	// mark is the marker request of the fill's caller, or nil. The drain writes
	// on behalf of several callers, so each fill marks for its own.
	mark *kvtx.BlockMarkRequest
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
// until the queue is empty, so no caller works through the fills of others.
// The drain uses background admission, since a fill is a copy the next read
// can redo. A caller whose context ends stops waiting; its fill is
// still written.
func (v *Volume) prepareCacheFill(ctx context.Context, owner string, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Queue the fill, and drain the queue if no caller is.
	fill := &cacheFill{owner: owner, data: data, opts: opts, mark: kvtx.GetBlockMarkRequest(ctx), done: make(chan struct{})}
	q := &v.fills
	q.mu.Lock()
	q.pending = append(q.pending, fill)
	drain := !q.draining
	q.draining = true
	q.mu.Unlock()
	if drain {
		go v.drainCacheFills(block.WithBackground(kvtx.WithBlockMarkRequest(context.WithoutCancel(ctx), nil)))
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
	if err != nil && len(fills) > 1 && !errors.Is(err, volume.ErrVolumeClosed) {
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
	// Name each fill's block and refuse new data below the disk reserve.
	size := 0
	direct := v.withDirectFill
	entries := make([]*block.PutBatchEntry, len(fills))
	for i, fill := range fills {
		size += len(fill.data)
		if fill.mark != nil {
			direct = v.withDirectAtomic
		}
		entry, err := putEntry(fill.data, fill.opts, v.GetHashType())
		if err != nil {
			return err
		}
		entries[i] = entry
	}
	if err := v.checkReserve(ctx, size); err != nil {
		return err
	}

	// Store the fills, each through its caller's marker, without the store's
	// writer unless one marks its block. A dropped fill stores nothing and
	// marks nothing.
	dropped := make([]block_gc.FillDrops, len(fills))
	err := direct(ctx, func(p *physicalTx) error {
		for i, fill := range fills {
			existed, drops, err := v.landFill(kvtx.WithBlockMarkRequest(ctx, fill.mark), p, owner, entries[i:i+1])
			if err != nil {
				return err
			}
			fill.ref, fill.existed, dropped[i] = entries[i].Ref, existed[0], drops
		}
		return nil
	})

	// Count the blocks they stored and dropped, or the refusal. A dropped
	// fill's caller gets block.ErrRefsUnknown: the block is not stored, and
	// the caller must not count it resident, though a later read can redo it.
	if err == nil {
		for i, fill := range fills {
			if len(dropped[i].At) != 0 {
				v.countDropped(putFill, dropped[i])
				fill.err = refsUnknown(fill.ref, len(fill.data))
				continue
			}
			v.countPut(putFill, len(fill.data), fill.existed)
		}
	}
	v.countRefused(putFill, err)
	return err
}

// prepareFillBatch writes a batch of cache fills that owner's bucket read, such
// as a pack's neighbors, in one fill transaction, holding each block as
// landFill does. The caller already batched the fills, so they skip the fill
// queue; the batch runs with background admission, since a fill is a copy the
// next read can redo. When it drops some entries, it returns a
// block.FillsDroppedError naming them with the other entries' results.
func (v *Volume) prepareFillBatch(ctx context.Context, owner string, entries []*block.PutBatchEntry) ([]bool, error) {
	// Refuse new data below the disk reserve. RefillBlocks drops the copied
	// entries of blocks the records do not list.
	v.noteUnknownRefsBatch(putFill, entries)
	if err := v.checkReserve(ctx, v.gateBytes(batchBytes(entries))); err != nil {
		return nil, err
	}

	// Store the fills under the bucket's fill roots, through the caller's
	// marker if it has one, and count them.
	direct := v.withDirectFill
	if kvtx.GetBlockMarkRequest(ctx) != nil {
		direct = v.withDirectAtomic
	}
	ctx = block.WithBackground(ctx)

	// Land the fills in one transaction, keeping the drops for the counts.
	var existed []bool
	var drops block_gc.FillDrops
	err := direct(ctx, func(p *physicalTx) error {
		var err error
		existed, drops, err = v.landFill(ctx, p, owner, entries)
		return err
	})

	// Count what was stored, dropped and refused. The dropped entries are the
	// batch's result, as a dropped single fill's is block.ErrRefsUnknown.
	v.countRefused(putFill, err)
	if err != nil {
		return existed, err
	}
	v.countPutBatch(putFill, withoutEntries(entries, drops.At), existed)
	v.countDropped(putFill, drops)
	if len(drops.At) != 0 {
		return existed, &block.FillsDroppedError{At: drops.At}
	}
	return existed, nil
}

// withoutEntries returns entries with those at the indexes at, which are
// ascending, replaced by tombstones, which count as nothing stored. It
// returns entries itself when at is empty.
func withoutEntries(entries []*block.PutBatchEntry, at []int) []*block.PutBatchEntry {
	if len(at) == 0 {
		return entries
	}
	out := slices.Clone(entries)
	for _, i := range at {
		out[i] = &block.PutBatchEntry{Tombstone: true}
	}
	return out
}

// notCacheFill reports whether entry is not a cache fill.
func notCacheFill(entry *block.PutBatchEntry) bool {
	return !entry.CacheFill
}

// fillRootPrefix prefixes the leased roots that hold the blocks a cache fill
// gives birth to, in the Volume's bucket with the empty ID. A fill root's name
// is the prefix, the bucket the fill came through, a slash and a ULID.
const fillRootPrefix = "fill:"

// landFill stores the entries of a cache fill through p, a copy of blocks a
// peer or remote store holds, which owner's bucket read. A block the records
// list gets its bytes back alone, through block_gc.RefillBlocks. The rest land
// through Land as a partial tree, so a block may name a child the settle
// hides, with a root on each block no other entry names under a fill lease
// with at least the collector's grace period left: the fill holds what it
// gives birth to for a grace period, and the collector frees it once the
// lease lapses unless something else holds it. It returns, for each entry,
// whether blocks held its bytes before, and the copied entries
// RefillBlocks dropped.
//
// RefillBlocks reads each block's child list, and Land the state record and
// the log positions, so an optimistic fill conflicts with each commit that
// lands before it; the fill transaction falls back to the store's writer
// after repeated conflicts.
func (v *Volume) landFill(ctx context.Context, p *physicalTx, owner string, entries []*block.PutBatchEntry) ([]bool, block_gc.FillDrops, error) {
	// Refill the listed blocks' bytes, dropping the copied entries of the
	// others.
	rest, existed, drops, err := block_gc.RefillBlocks(ctx, p.rec, p.blocks, entries)
	if err != nil || len(rest) == 0 {
		return existed, drops, err
	}

	// Join a fill lease with at least the fill hold left, when a block of
	// the rest has no other name.
	hold := v.fillHold
	if hold == 0 {
		hold = block_gc.DefaultGrace
	}
	refs := unnamedRefs(rest)
	var lease string
	if len(refs) != 0 {
		if lease, err = v.fillLease(ctx, p, hold); err != nil {
			return nil, drops, err
		}
	}

	// Hold each such block under the lease, then land the rest.
	roots := make([]block.NamedRoot, 0, len(refs))
	for _, ref := range refs {
		roots = append(roots, block.NamedRoot{Name: fillRootPrefix + owner + "/" + ulid.NewULID(), Ref: ref, Lease: lease})
	}
	puts := block_gc.PutHold{Mode: v.heldMode, Observed: v.observeNotHeld, Partial: true}
	landed, err := block_gc.Land(ctx, p.rec, p.blocks, v.now(), rest, puts, block_gc.BucketRoots{Roots: roots})
	if err != nil {
		return nil, drops, err
	}

	// Merge the landed entries' existence back in entry order.
	j := 0
	for i, entry := range entries {
		if j < len(rest) && entry == rest[j] {
			existed[i] = j < len(landed) && landed[j]
			j++
		}
	}
	return existed, drops, nil
}

// unnamedRefs returns the refs of the entries no other entry names, once
// each, in entry order.
func unnamedRefs(entries []*block.PutBatchEntry) []*block.BlockRef {
	// Note every name, then keep the entries none names.
	named := make(map[string]struct{})
	for _, entry := range entries {
		for _, ref := range entry.Refs {
			named[ref.MarshalString()] = struct{}{}
		}
	}
	var refs []*block.BlockRef
	for _, entry := range entries {
		key := entry.Ref.MarshalString()
		if _, ok := named[key]; ok {
			continue
		}
		named[key] = struct{}{}
		refs = append(refs, entry.Ref)
	}
	return refs
}
