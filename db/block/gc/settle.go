package block_gc

import (
	"bytes"
	"context"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// The settle of an old generation.
//
// A Volume whose child lists came from a store format before 7 holds lists
// that may lack a child: the old reference graph wrote a block's edges in
// batches, and a crash between two batches left the list cut. A count drawn
// from a cut list misses a holder, so a free could take a block a live block
// still names. The settle proves which old blocks are live before anything
// frees, as Journal.tla's WalkStart, WalkStep, FrontFree, WalkEnd and
// SweepDel actions model it. The generation, in the state record, moves
// Open, Sweeping, Settled, and never back.
//
// A list is young or verified when it was written from its block's content:
// by a Land or a fill, or by the typed walk, which decodes the block. Its
// value begins with listMark. Every other list is unverified.
//
// Invariants, while the generation is Open:
//
//   - Nothing is deleted: the free parks each block it would free.
//   - The front holds every unverified listed child of a young or verified
//     block. Land and LandFill push the unverified children of each block they
//     write from content, and each unverified block a root moves onto; the walk
//     pushes the unverified children of each block it decodes. So the walk
//     never descends below a verified block, and no young block needs seeding.
//   - Once its completion entry exists, the unmarked run names every block
//     whose list is unverified: the run build adds each, and every write that
//     marks a list deletes the block's entry in the same transaction. No
//     writer adds an unverified list, since PutChildren always marks.
//
// The flip is one state write, to Sweeping, in a transaction that finds the
// Volume adopted with every cascade and fill up netted, no list rewrite
// awaiting a recount, no recorded hole, the run complete and the front empty,
// and that pushes every unverified block a named root or a down pair past
// the cursor names, flipping only when it pushed none. That is the model's
// WalkStart followed at once by WalkEnd. Every block still unverified is then
// unreachable: no seed names it, and no young or verified block does, since
// the front is empty. The run names exactly those blocks.
//
// While Sweeping, an unmarked block is one the run names with an empty
// entry, or one that was Dead at the flip, which every site that asks
// whether a block is stored answers absent, as Unmarked describes. The flip, which requires no cascade due, freezes each
// unmarked block's count record, which says whether its old list still holds
// its children,
// and counts its holders from the flip on apart, as kindUnmarkedCount
// describes, so no later write changes what the old list holds. The sweep
// deletes each unmarked block, releasing the holder its old list gave each
// child outside the run when it still holds, as releaseOld decides. A put
// of an unmarked block gives birth to it again: its entry turns reborn, its
// old list releases the same way, and it keeps the holders it gained since
// the flip. The sweep marks each entry it deletes swept rather than
// deleting it, so the run stays the run at the flip. Once no unmarked block
// is left, the sweep deletes the run and the state settles.

// AbsentError is returned by a Land that holds a hidden block, as
// Records.Hidden describes: one the settle unmarked, which no live block
// names and the sweep will delete, or one the free parked while Open. The
// writer puts it again, as for a block the store holds without a child list.
type AbsentError struct {
	// Ref is the block.
	Ref *block.BlockRef
}

// Error describes the block.
func (e *AbsentError) Error() string {
	return "block " + e.Ref.MarshalString() + " reads absent until it is put again"
}

// Unwrap returns block.ErrRefsUnknown, which tells a writer to put the
// block again.
func (e *AbsentError) Unwrap() error {
	return block.ErrRefsUnknown
}

// Unmarked reports whether the block at key is unmarked: the generation is
// Sweeping, and either the run names the block with an entry neither reborn
// nor swept, or the block has no run entry and was Dead at the flip, its
// count record zero at a ripe sequence no later than the flip's cursor.
// Outside Sweeping it reads nothing more than the generation; while
// Sweeping, the block's run entry and, without one, its count record.
//
// Invariant, while Sweeping: a block that does not read unmarked lists no
// block the sweep deleted, unless a put stored that block again. The flip
// leaves no unverified block a seed or a young or verified block names,
// except one the front popped: Dead at a ripe sequence the cursor passed. A
// marked block that lists a popped child held it, so it released the child
// when it reached zero itself, at a ripe sequence no later than the child's
// (the audit checks this order, Journal.tla's PoppedRipe). The flip
// requires its cursor at or past the hold mark, so no Land past the cursor
// held either, and both were Dead at the flip and read unmarked together. A
// Land that holds an unmarked block gets AbsentError, and the writer's put
// gives birth to it again with a list from content, as rebirth describes.
//
// A block that reaches zero after the flip never joins the run this way.
// Netting is the only writer of a zero count while Sweeping, and it gives a
// block that reaches zero, or that a record holds again or puts again, the
// next journal sequence as its ripe, past the record it nets; every record
// netted after the flip is at or past the flip's cursor. The adoption, the
// other writer of a ripe, runs only on an unadopted Volume, and nothing
// unadopts a Sweeping one: the flip requires it adopted, a claim takes only
// one that is not, and the flip's return to unadopted runs only while Open.
//
// A block born after the flip that names a Dead marked block past the grace
// period follows the grace contract: nothing held the named block when its
// grace ended, so the Land refuses it and the writer puts it again.
//
// The flip is the only writer of Sweeping, and it writes the state's Flip
// with it; a reachable format copy refuses a Sweeping source. A Sweeping
// state without a Flip answers by its run entries alone, and the collector
// deletes nothing in it, as Collector.settle describes.
func (r *Records) Unmarked(ctx context.Context, key []byte) (bool, error) {
	// Read the generation, then the run entry.
	gen, err := r.generation(ctx)
	if err != nil || gen != Generation_GENERATION_SWEEPING {
		return false, err
	}
	value, found, err := r.tx.Get(ctx, r.key(kindUnmarked, key))
	if err != nil || found || r.flip == nil {
		return found && len(value) == 0, err
	}

	// Without one, read whether the block was Dead at the flip.
	value, found, err = r.tx.Get(ctx, r.key(kindCount, key))
	if err != nil || !found {
		return false, err
	}
	holders, pos, err := decodeHolders(value)
	return holders == 0 && pos.ripe <= r.flip.GetCursor(), err
}

// countKind returns the kind of the count record of the block at key:
// kindUnmarkedCount while it is unmarked, since the flip froze its kindCount
// record, and kindCount otherwise. Outside Sweeping it reads nothing more
// than the generation; while Sweeping, one run entry.
func (r *Records) countKind(ctx context.Context, key []byte) (byte, error) {
	unmarked, err := r.Unmarked(ctx, key)
	if err != nil || !unmarked {
		return kindCount, err
	}
	return kindUnmarkedCount, nil
}

// oldListHolds reports whether the old list of the unmarked block at key
// still holds its children, from its kindCount record as the flip froze it:
// a held block's children count it, and a Dead one's cascade already
// released them, so releasing them again would take a holder from a child
// something else still holds. No cascade is due on an unmarked block: the
// flip requires none due, and netting writes none for an unmarked block,
// which it reads as unlisted.
func (r *Records) oldListHolds(ctx context.Context, key []byte) (bool, error) {
	value, found, err := r.tx.Get(ctx, r.key(kindCount, key))
	if err != nil || !found {
		return err == nil, err
	}
	holders, _, err := decodeHolders(value)
	return holders != 0, err
}

// releaseOld ends the old generation's records of the unmarked block at
// key as it leaves the run, by the sweep or a rebirth, and returns the block
// keys of the children outside the run its old list still holds, as
// oldListHolds decides, which lose that holder. It moves its
// kindUnmarkedCount record, the holders it gained and lost since the flip,
// over its frozen kindCount record. A child
// inside the run keeps no holder from it, since the flip dropped the
// holders of every unmarked block.
func (r *Records) releaseOld(ctx context.Context, key []byte) ([][]byte, error) {
	// Read whether the old list holds.
	holds, err := r.oldListHolds(ctx, key)
	if err != nil {
		return nil, err
	}
	var released [][]byte
	if holds {
		value, _, err := r.tx.Get(ctx, r.key(kindChildren, key))
		if err != nil {
			return nil, err
		}
		if released, err = r.outsideRun(ctx, bytes.Clone(listKeys(value))); err != nil {
			return nil, err
		}
	}

	// Move the count since the flip over the frozen one.
	count, found, err := r.tx.Get(ctx, r.key(kindUnmarkedCount, key))
	switch {
	case err != nil:
		return nil, err
	case !found:
		return released, r.tx.Delete(ctx, r.key(kindCount, key))
	}
	if err := r.tx.Set(ctx, r.key(kindCount, key), bytes.Clone(count)); err != nil {
		return nil, err
	}
	return released, r.tx.Delete(ctx, r.key(kindUnmarkedCount, key))
}

// listState reads the list record of the block at key: whether it exists,
// and whether it was written from content.
func (r *Records) listState(ctx context.Context, key []byte) (listed, marked bool, err error) {
	value, found, err := r.tx.Get(ctx, r.key(kindChildren, key))
	return found, found && listMarked(value), err
}

// pushUnverified pushes the block at key onto the front when its list is
// unverified, and reports whether it did.
func (r *Records) pushUnverified(ctx context.Context, key []byte) (bool, error) {
	listed, marked, err := r.listState(ctx, key)
	if err != nil || !listed || marked {
		return false, err
	}
	return true, r.tx.Set(ctx, r.key(kindFront, key), nil)
}

// returnHeld keeps the rule for a block that gains a holder while Open: its
// list unverified, it returns to the front, since the free proof may have
// popped it while it was Dead, and the walk must verify it before the flip.
// The re-hold of a cascade and the recount of a readoption both call it. It
// reports whether the block has a list and whether it pushed it; outside Open
// it only reads the list.
func (r *Records) returnHeld(ctx context.Context, key []byte, open bool) (listed, pushed bool, err error) {
	listed, marked, err := r.listState(ctx, key)
	if err != nil || !listed || marked || !open {
		return listed, false, err
	}
	return true, true, r.tx.Set(ctx, r.key(kindFront, key), nil)
}

// pushChildren pushes each unverified child in packed, block keys as a list
// holds them, onto the front, skipping a child skip reports.
func (r *Records) pushChildren(ctx context.Context, packed []byte, skip func(key []byte) bool) error {
	for len(packed) != 0 {
		key, ok := nextBlockKey(packed)
		if !ok {
			return errBadBlockKey
		}
		packed = packed[len(key):]
		if skip != nil && skip(key) {
			continue
		}
		if _, err := r.pushUnverified(ctx, key); err != nil {
			return err
		}
	}
	return nil
}

// markedYoung finishes a write that marked the list of the block at key,
// packed its children, while the generation is Open: the block leaves the
// run and the front, and its unverified children join the front, which keeps
// the front invariant.
func (r *Records) markedYoung(ctx context.Context, key, packed []byte, skip func(key []byte) bool) error {
	if err := r.tx.Delete(ctx, r.key(kindUnmarked, key)); err != nil {
		return err
	}
	if err := r.tx.Delete(ctx, r.key(kindFront, key)); err != nil {
		return err
	}
	return r.pushChildren(ctx, packed, skip)
}

// mergeList writes the union of the unverified list value of the block at
// key and refs, its content's refs, marked, and returns the union's block
// keys and whether the list gained a child. With strict, refs that lack a
// child the list names write nothing and report dropped: a stored list can
// only miss children, so such refs come from a wrong decoder. A list that
// gains a child journals no holder for it, so the caller records that the
// counts owe a recount.
func (r *Records) mergeList(ctx context.Context, key, value []byte, refs []*block.BlockRef, strict bool) (packed []byte, gained, dropped bool, err error) {
	// Refuse refs that drop a child, then write the marked union.
	old, encoded := listKeys(value), EncodeChildren(refs)
	if strict && !containsChildren(encoded, old) {
		return nil, false, true, nil
	}
	union, err := unionChildren(old, encoded)
	if err != nil {
		return nil, false, false, err
	}
	gained = !bytes.Equal(union, old)
	return union, gained, false, r.tx.Set(ctx, r.key(kindChildren, key), markList(union))
}

// noteRewrite records, on a Volume that keeps a journal, that a list gained
// a child no holder counts, so the flip returns the Volume to unadopted and
// an adoption recounts the lists first.
func (r *Records) noteRewrite(ctx context.Context, state *LivenessState) error {
	if state.GetAdoption() == Adoption_ADOPTION_UNADOPTED {
		return nil
	}
	return r.tx.Set(ctx, r.key(kindRewritten), nil)
}

// rebirth gives birth again to the unmarked block ref while the generation
// is Sweeping: it leaves the run as releaseOld describes, keeping the
// holders it gained since the flip, its run entry turns reborn, and it
// returns the block keys of the children its old list still held, which
// lose that holder. The caller writes the new list.
func (r *Records) rebirth(ctx context.Context, ref *block.BlockRef) ([][]byte, error) {
	key := BlockKey(ref)
	released, err := r.releaseOld(ctx, key)
	if err != nil {
		return nil, err
	}
	return released, r.tx.Set(ctx, r.key(kindUnmarked, key), runReborn)
}

// outsideRun returns the block keys in packed that have no run entry: the
// blocks outside the run at the flip, since no entry is deleted while
// Sweeping. A child the sweep deleted and a put stored again stays inside,
// since its new count never had the old list's holder.
func (r *Records) outsideRun(ctx context.Context, packed []byte) ([][]byte, error) {
	var out [][]byte
	for len(packed) != 0 {
		key, ok := nextBlockKey(packed)
		if !ok {
			return nil, errBadBlockKey
		}
		packed = packed[len(key):]
		inRun, err := r.tx.Exists(ctx, r.key(kindUnmarked, key))
		if err != nil {
			return nil, err
		}
		if !inRun {
			out = append(out, key)
		}
	}
	return out, nil
}

// settle runs the settle's steps of one collector pass while frees are on,
// each in slices: while Open, it builds the run, takes the free-proven
// entries off the front, and flips when the flip's checks pass; while
// Sweeping, it sweeps. It reports whether it stopped at past with work left.
//
// A Sweeping state without a Flip cannot say which blocks were Dead at the
// flip, so the pass stalls: it sweeps nothing and the free frees nothing,
// and it reports StallNoFlip. No flip of this format writes that state.
func (c *Collector) settle(ctx context.Context, past func() bool, res *CollectResult) (bool, error) {
	// Read the generation and the flip.
	var state *LivenessState
	err := c.store.View(ctx, func(rec *Records) error {
		var err error
		state, err = rec.GetState(ctx)
		return err
	})
	gen := state.GetGeneration()
	if err != nil || gen == Generation_GENERATION_SETTLED {
		return false, err
	}
	if gen == Generation_GENERATION_SWEEPING && state.GetFlip() == nil {
		res.Stall = StallNoFlip
		return false, nil
	}

	// Sweep a flipped Volume: one pass from the start of the run that
	// reaches its end deletes every unmarked block the run names, since
	// none is added while Sweeping. The parked slice sweeps the parked
	// blocks, and the free the queued ones, that were Dead at the flip, and
	// once none is left the run goes and the state settles.
	if gen == Generation_GENERATION_SWEEPING {
		if !c.sweptRun {
			var reached bool
			more, err := eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, blocks block.StoreOps, from []byte) ([]byte, bool, error) {
				next, more, end, err := c.sweepSlice(ctx, rec, blocks, res, from)
				reached = end
				return next, more, err
			})
			if err != nil || more || !reached {
				return more, err
			}
			c.sweptRun = true
		}
		more, err := eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, blocks block.StoreOps, from []byte) ([]byte, bool, error) {
			return c.parkedSlice(ctx, rec, blocks, res, from)
		})
		if err != nil || more {
			return more, err
		}
		return eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, _ block.StoreOps, from []byte) ([]byte, bool, error) {
			return c.dropRunSlice(ctx, rec, res, from)
		})
	}

	// Build the run, resuming where the last pass stopped, take the
	// free-proven entries off the front, then try the flip.
	var reached []byte
	more, err := eachSlice(ctx, c.update, c.runFrom, past, func(rec *Records, _ block.StoreOps, from []byte) ([]byte, bool, error) {
		next, more, err := c.runSlice(ctx, rec, res, from)
		reached = next
		return next, more, err
	})
	if err != nil {
		return false, err
	}
	c.runFrom = reached
	if more {
		return more, nil
	}
	more, err = eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, _ block.StoreOps, from []byte) ([]byte, bool, error) {
		return c.frontSlice(ctx, rec, res, from)
	})
	if err != nil || more {
		return more, err
	}
	return false, c.update(ctx, func(rec *Records, _ block.StoreOps) error {
		return c.flip(ctx, rec, res)
	})
}

// runSlice adds a run entry for each unverified list of up to a slice of
// child list records after the key position from, while the generation is
// Open, and writes the run's completion entry once it passes the last.
//
// Invariant: no write while Open stores an unmarked list, so a list unmarked
// at the flip was unmarked whenever the scan passed it, and the run at the
// flip is every list still unmarked, whatever writes interleave with the
// scan. Every writer of kindChildren while Open marks: PutChildren, the union
// rewrite and ListRebuild. A writer that stores an unmarked list while Open
// breaks the run.
//
// Cost: one sequential read of the list records, values included, and one
// write per unverified list, once per Volume.
func (c *Collector) runSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Build only while Open, and once.
	gen, err := rec.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return from, false, err
	}
	done, err := rec.tx.Exists(ctx, rec.key(kindUnmarked, runComplete))
	if err != nil || done {
		return from, false, err
	}

	// Take the next lists within the slice's budget.
	seek := from
	if from != nil {
		seek = append(bytes.Clone(from), 0)
	}
	budget, n, full := c.share(sliceRecords), 0, false
	var unverified [][]byte
	err = rec.scan(ctx, rec.key(kindChildren), seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Stop at a full slice; note each unmarked list.
		if n == budget {
			full = true
			return false, nil
		}
		n++
		from = bytes.Clone(key)
		v, err := value()
		if err == nil && !listMarked(v) {
			unverified = append(unverified, from)
		}
		return true, err
	})
	if err != nil {
		return from, false, err
	}

	// Enter each unmarked list in the run, and complete the run past the last.
	for _, key := range unverified {
		if err := rec.tx.Set(ctx, rec.key(kindUnmarked, key), nil); err != nil {
			return from, false, err
		}
	}
	res.Unmarked += uint64(len(unverified))
	if full {
		return from, true, nil
	}
	return from, false, rec.tx.Set(ctx, rec.key(kindUnmarked, runComplete), nil)
}

// frontSlice takes off the front each of up to a slice of entries after the
// key position from whose block the free proof holds for, while the
// generation is Open: Dead at a ripe sequence the cursor passed, with no
// cascade due and every fill up applied, as the free checks. Nothing holds
// such a block or can within the grace period. Its count misses only a
// holder whose list is unverified and short; that holder, if anything holds
// it, is a seed the flip pushes or a block below a front entry, which holds
// the flip until the walk decodes it. A list that gained a child journaled
// no holder for it, so while kindRewritten exists the counts may miss a
// holder whose list is marked, and nothing pops. The record lasts until the
// next flip attempt returns the Volume to unadopted and the adoption that
// follows deletes it, so the front waits for one recount at most. A block
// popped before the rewrite that the rewritten list names gains a holder in
// that recount, which returns it to the front, as returnHeld describes.
func (c *Collector) frontSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Pop only while Open, and while the free proof can hold.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN {
		return from, false, err
	}
	if ok, err := rec.freeable(ctx, state); err != nil || !ok {
		return from, false, err
	}
	if recount, err := rec.tx.Exists(ctx, rec.key(kindRewritten)); err != nil || recount {
		return from, false, err
	}

	// Take the next entries.
	seek := from
	if from != nil {
		seek = append(bytes.Clone(from), 0)
	}
	budget, full := c.share(sliceRecords), false
	var keys [][]byte
	err = rec.scan(ctx, rec.key(kindFront), seek, func(key []byte, _ func() ([]byte, error)) (bool, error) {
		if len(keys) == budget {
			full = true
			return false, nil
		}
		keys = append(keys, bytes.Clone(key))
		return true, nil
	})
	if err != nil || len(keys) == 0 {
		return from, false, err
	}

	// Pop each the proof holds for.
	cursor := state.GetCursor()
	for _, key := range keys {
		from = key
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return from, false, err
		}
		listed, err := rec.HasChildren(ctx, ref)
		if err != nil {
			return from, false, err
		}
		holders, pos, err := rec.GetHolders(ctx, ref)
		if err != nil {
			return from, false, err
		}
		if !listed || holders != 0 || pos.ripe > cursor || cursor < state.GetHoldMark() {
			continue
		}
		if err := rec.tx.Delete(ctx, rec.key(kindFront, key)); err != nil {
			return from, false, err
		}
		res.Popped++
	}
	return from, full, nil
}

// flip moves an Open Volume to Sweeping when the flip's checks pass, as the
// settle describes, or, when a list rewrite awaits a recount, returns the
// Volume to unadopted. It pushes each unverified seed it finds, which holds
// the flip until the walk decodes it.
//
// Cost: one point read of the run's completion entry and three prefix
// seeks; when those pass, every named root and every journal record past the
// cursor with each block it names in a down pair, one list read each, in one
// transaction, once per collector pass until the flip.
func (c *Collector) flip(ctx context.Context, rec *Records, res *CollectResult) error {
	// Require an Open, adopted Volume with every cascade and fill up netted.
	// Collect nets to completion before the settle runs, and only netting
	// writes a cascade or a paged release, so today the cascade half never
	// refuses; it is the model's FlipSettled, kept because a step that leads
	// to deletes waits when unsure.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN {
		return err
	}
	if ok, err := rec.freeable(ctx, state); err != nil || !ok {
		return err
	}

	// Wait for the cursor to reach the hold mark: a Land past the cursor
	// may hold a block at zero, which the flip would read as Dead. Netting
	// takes a record only once it is Grace old, so holds of zero blocks
	// arriving with no gap of Grace hold the flip off, as they do every
	// free, for as long as they arrive; the writers wait on nothing.
	if state.GetCursor() < state.GetHoldMark() {
		res.Stall = StallHoldMark
		return nil
	}

	// A rewritten list's new children lack their holders: recount first.
	// This is the settle's one return to unadopted, and it runs only while
	// Open. An adoption while Sweeping would recount the unmarked blocks'
	// frozen kindCount records and clear their kindUnmarkedCount records,
	// losing whether each old list still holds its children and the holders
	// gained since the flip; a future trigger of a readopt must wait for
	// Settled, or recount into kindUnmarkedCount.
	recount, err := rec.tx.Exists(ctx, rec.key(kindRewritten))
	if err != nil {
		return err
	}
	if recount {
		rec.unadopt(state)
		res.Unadopted = true
		return rec.SetState(ctx, state)
	}

	// Require the run complete, the front empty and no recorded hole.
	done, err := rec.tx.Exists(ctx, rec.key(kindUnmarked, runComplete))
	if err != nil || !done {
		return err
	}
	for _, kind := range []byte{kindFront, kindHole} {
		if found, err := rec.anyKey(ctx, rec.key(kind)); err != nil || found {
			return err
		}
	}

	// Push each unverified seed: a named root's block, and each block a down
	// pair past the cursor names, which a reader or writer may still hold.
	var seeded uint64
	err = rec.ScanRoots(ctx, func(_, _ string, entry *RootEntry) error {
		pushed, err := rec.pushUnverified(ctx, entry.GetRef())
		if pushed {
			seeded++
		}
		return err
	})
	if err != nil {
		return err
	}
	err = rec.ScanJournal(ctx, state.GetCursor(), func(_ uint64, jr *JournalRecord) (bool, error) {
		for _, key := range jr.GetDown() {
			pushed, err := rec.pushUnverified(ctx, key)
			if err != nil {
				return false, err
			}
			if pushed {
				seeded++
			}
		}
		return true, nil
	})
	if err != nil || seeded != 0 {
		res.Seeded += seeded
		return err
	}

	// Flip, recording the cursor that says which blocks were Dead.
	state.Generation = Generation_GENERATION_SWEEPING
	state.Flip = &Flip{Cursor: state.GetCursor(), Order: state.GetNextOrder()}
	res.Flipped = true
	return rec.SetState(ctx, state)
}

// sweepBlock deletes the unmarked block ref at key: its bytes, list and
// proof, and marks its run entry swept, which a block Dead at the flip
// gains here. Its count keeps the holders it gained since the flip, as a
// free keeps a block's netted holders. It returns the block keys of the
// children its old list still holds, as releaseOld decides, which the
// caller journals a down pair for each, as a free's cascade releases them;
// a block Dead at the flip has none, since its cascade released them. A
// child inside the run at the flip keeps no holder from the old list, so
// the sweep of an unmarked parent releases nothing to it, even after a put
// stored it again. The run sweep, the parked slice and the free call it.
func (c *Collector) sweepBlock(ctx context.Context, rec *Records, blocks block.StoreOps, ref *block.BlockRef, key []byte, res *CollectResult) ([][]byte, error) {
	// Release the old list, then delete the block and mark it swept.
	children, err := rec.releaseOld(ctx, key)
	if err != nil {
		return nil, err
	}
	if err := blocks.RmBlock(ctx, ref); err != nil {
		return nil, err
	}
	for _, del := range []func(context.Context, *block.BlockRef) error{rec.DeleteChildren, rec.DeleteProof} {
		if err := del(ctx, ref); err != nil {
			return nil, err
		}
	}
	res.Swept++
	return children, rec.tx.Set(ctx, rec.key(kindUnmarked, key), runSwept)
}

// sweepUnmarked sweeps ref, as sweepBlock describes, when it has a list
// and reads unmarked, and reports whether it did, with the children its old
// list released.
//
// Cost: the list read and Unmarked's reads, then the sweep.
func (c *Collector) sweepUnmarked(ctx context.Context, rec *Records, blocks block.StoreOps, ref *block.BlockRef, res *CollectResult) (bool, [][]byte, error) {
	// Sweep only a listed block that reads unmarked.
	key := BlockKey(ref)
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil || !listed {
		return false, nil, err
	}
	unmarked, err := rec.Unmarked(ctx, key)
	if err != nil || !unmarked {
		return false, nil, err
	}
	children, err := c.sweepBlock(ctx, rec, blocks, ref, key, res)
	return true, children, err
}

// sweepSlice sweeps each unmarked block of up to a slice of run entries
// after the key position from, as sweepBlock describes, while the
// generation is Sweeping and the Volume adopted, and journals the down
// pairs. It skips a reborn or swept entry. It reports whether it reached
// the run's end.
//
// Cost: per unmarked block, its two count reads and cascade read, its list
// read, the deletes, and one run entry read per child; per slice, one
// journal record and the state write.
func (c *Collector) sweepSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from []byte) ([]byte, bool, bool, error) {
	// Sweep only an adopted Volume, since the down pairs need its journal.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_SWEEPING {
		return from, false, false, err
	}
	if state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return from, false, false, nil
	}

	// Take the next entries.
	keys, unmarked, full, err := c.runEntries(ctx, rec, from, sliceFrees)
	if err != nil {
		return from, false, false, err
	}

	// Delete each unmarked block, and journal the release of its children.
	var down [][]byte
	for i, key := range keys {
		from = key
		if !unmarked[i] {
			continue
		}
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return from, false, false, err
		}
		children, err := c.sweepBlock(ctx, rec, blocks, ref, key, res)
		if err != nil {
			return from, false, false, err
		}
		down = append(down, children...)
	}
	if len(down) != 0 {
		return from, full, !full, rec.appendDowns(ctx, state, c.opts.Now(), down)
	}
	return from, full, !full, rec.SetState(ctx, state)
}

// parkedSlice takes up to a slice of the parked keys after the key
// position from while the generation is Sweeping under a Flip and the Volume
// adopted, and sweeps each block still unmarked, as sweepBlock describes.
// The free parked each while Open, at a ripe sequence the cursor had
// passed, so each still at zero at that sequence was Dead at the flip. It
// deletes each key; one whose block a put gave birth to again, or whose
// count moved, sweeps nothing. Nothing parks while Sweeping, so the keys
// only shrink. It reports whether the slice was full.
//
// Cost: per key, its list and count reads, the run entry read, and for an
// unmarked block the sweep; per slice, the state write and any journal
// record.
func (c *Collector) parkedSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Sweep only an adopted Volume under a Flip.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_SWEEPING || state.GetFlip() == nil {
		return from, false, err
	}
	if state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return from, false, nil
	}

	// Take the next parked entries.
	var parked []queueEntry
	full, budget := false, c.share(sliceFrees)
	err = rec.scanDead(ctx, kindParked, from, func(pos DeadPos, ref *block.BlockRef) (bool, error) {
		if len(parked) == budget {
			full = true
			return false, nil
		}
		parked = append(parked, queueEntry{pos: pos, ref: ref})
		return true, nil
	})
	if err != nil {
		return from, false, err
	}

	// Delete each key, sweeping its block while it is unmarked.
	var down [][]byte
	for _, e := range parked {
		key := BlockKey(e.ref)
		from = queuePos(e.pos, key)
		if err := rec.tx.Delete(ctx, rec.parkedKey(e.pos, key)); err != nil {
			return from, false, err
		}
		_, children, err := c.sweepUnmarked(ctx, rec, blocks, e.ref, res)
		if err != nil {
			return from, false, err
		}
		down = append(down, children...)
	}
	if len(down) != 0 {
		return from, full, rec.appendDowns(ctx, state, c.opts.Now(), down)
	}
	return from, full, nil
}

// deadLeft reports whether a block Dead at the flip may be left to sweep
// under state: a parked key, or a queue entry ordered before the flip, at
// a ripe sequence no later than the flip's cursor. The flip's Dead blocks
// each hold one or the other, and nothing adds either at such a place
// while Sweeping: a cascade since the flip takes a later order number, and
// a block that ripens again takes a later ripe sequence.
//
// Cost: two prefix seeks, then one key per entry ordered before the flip
// that ripened again since, up to the first one left.
func (r *Records) deadLeft(ctx context.Context, state *LivenessState) (bool, error) {
	// A parked key is left to sweep.
	parked, err := r.anyKey(ctx, r.key(kindParked))
	if err != nil || parked {
		return parked, err
	}

	// So is a queue entry ordered before the flip at a ripe sequence no
	// later than the flip's cursor; one ordered before it at a later ripe
	// sequence ripened again since, and was not Dead at the flip.
	flip := state.GetFlip()
	var left bool
	err = r.scanDead(ctx, kindQueue, nil, func(pos DeadPos, _ *block.BlockRef) (bool, error) {
		if pos.order >= flip.GetOrder() {
			return false, nil
		}
		left = pos.ripe <= flip.GetCursor()
		return !left, nil
	})
	return left, err
}

// dropRunSlice deletes up to a slice of run entries after the key position
// from, once a sweep pass left no unmarked block the run names and no block
// Dead at the flip is left, as deadLeft decides, and, past the last, deletes
// the completion entry, clears the Flip and settles the generation. A block
// Dead at the flip that a slice swept gained a swept entry first, so the
// drop deletes it too.
func (c *Collector) dropRunSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Drop only while Sweeping, once no Dead block is left.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_SWEEPING || state.GetFlip() == nil {
		return from, false, err
	}
	if left, err := rec.deadLeft(ctx, state); err != nil || left {
		return from, false, err
	}

	// Delete the next entries.
	keys, _, full, err := c.runEntries(ctx, rec, from, sliceRecords)
	if err != nil {
		return from, false, err
	}
	for _, key := range keys {
		from = key
		if err := rec.tx.Delete(ctx, rec.key(kindUnmarked, key)); err != nil {
			return from, false, err
		}
	}
	if full {
		return from, true, nil
	}

	// Past the last, delete the completion entry and settle.
	if err := rec.tx.Delete(ctx, rec.key(kindUnmarked, runComplete)); err != nil {
		return from, false, err
	}
	c.sweptRun = false
	state.Generation = Generation_GENERATION_SETTLED
	state.Flip = nil
	res.Settled = true
	return from, false, rec.SetState(ctx, state)
}

// runEntries returns the block keys of up to a slice of base run entries
// after the key position from, past the completion entry, whether each is
// still unmarked, neither reborn nor swept, and whether the slice was full.
func (c *Collector) runEntries(ctx context.Context, rec *Records, from []byte, base int) ([][]byte, []bool, bool, error) {
	// Scan past from, skipping the completion entry, until the slice fills.
	seek := from
	if from != nil {
		seek = append(bytes.Clone(from), 0)
	}
	budget := c.share(base)
	var keys [][]byte
	var unmarked []bool
	full := false
	err := rec.scan(ctx, rec.key(kindUnmarked), seek, func(key []byte, value func() ([]byte, error)) (bool, error) {
		// Skip the completion entry, stop at a full slice, and take the entry.
		if len(key) == 0 {
			return true, nil
		}
		if len(keys) == budget {
			full = true
			return false, nil
		}
		v, err := value()
		keys = append(keys, bytes.Clone(key))
		unmarked = append(unmarked, len(v) == 0)
		return true, err
	})
	return keys, unmarked, full, err
}

// appendDowns appends down, block keys losing one holder each, to the
// journal at now, in pages of journalPagePairs, advancing state's next
// sequence, and writes state.
func (r *Records) appendDowns(ctx context.Context, state *LivenessState, now time.Time, down [][]byte) error {
	for len(down) != 0 {
		n := min(len(down), journalPagePairs)
		page := &JournalRecord{AtUnixMilli: now.UnixMilli(), Down: down[:n]}
		if err := r.PutJournal(ctx, state.GetNextSeq(), page); err != nil {
			return err
		}
		state.NextSeq++
		down = down[n:]
	}
	return r.SetState(ctx, state)
}
