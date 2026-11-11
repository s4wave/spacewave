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
// While Sweeping, a run entry that is not reborn names an unmarked block,
// which every site that asks whether a block is stored answers absent, as
// Unmarked describes. The flip freezes each unmarked block's count and
// cascade records, which say whether its old list still holds its children,
// and counts its holders from the flip on apart, as kindUnmarkedCount
// describes, so no later write changes what the old list holds. The sweep
// deletes each unmarked block, releasing the holder its old list gave each
// child outside the run when it still holds, as releaseOld decides. A put
// of an unmarked block gives birth to it again: its entry turns reborn, its
// old list releases the same way, and it keeps the holders it gained since
// the flip. Once no unmarked block is left, the sweep deletes the run and
// the state settles.

// UnmarkedError is returned by a Land that holds a block the settle
// unmarked: no live block names it, and the sweep will delete it. The writer
// puts it again, as for a block the store holds without a child list.
type UnmarkedError struct {
	// Ref is the block.
	Ref *block.BlockRef
}

// Error describes the block.
func (e *UnmarkedError) Error() string {
	return "block " + e.Ref.MarshalString() + " was unmarked by the settle of the old generation; put it again"
}

// Unwrap returns block.ErrRefsUnknown, which tells a writer to put the
// block again.
func (e *UnmarkedError) Unwrap() error {
	return block.ErrRefsUnknown
}

// Unmarked reports whether the block at key is unmarked: the generation is
// Sweeping and the block's run entry is not reborn. It reads nothing more
// than the generation outside Sweeping.
func (r *Records) Unmarked(ctx context.Context, key []byte) (bool, error) {
	gen, err := r.generation(ctx)
	if err != nil || gen != Generation_GENERATION_SWEEPING {
		return false, err
	}
	value, found, err := r.tx.Get(ctx, r.key(kindUnmarked, key))
	return found && !bytes.Equal(value, runReborn), err
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
// still holds its children, from its kindCount and kindCascade records as
// the flip froze them, and whether a cascade is due on it. Its children
// count it exactly when its frozen holders and a due cascade disagree: a
// held block with none due, or a Dying one at zero whose down cascade is
// due. A Dead block at zero with none due already released them through its
// cascade, as did a Holding one whose up cascade is due, so releasing them
// again would take a holder from a child something else still holds.
func (r *Records) oldListHolds(ctx context.Context, key []byte) (holds, due bool, err error) {
	// Read the frozen count, then the frozen cascade.
	value, found, err := r.tx.Get(ctx, r.key(kindCount, key))
	if err != nil {
		return false, false, err
	}
	holds = true
	if found {
		holders, _, err := decodeHolders(value)
		if err != nil {
			return false, false, err
		}
		holds = holders != 0
	}
	dir, due, err := r.getCascade(ctx, key)
	if err != nil || !due {
		return holds, due, err
	}
	return dir == cascadeDown, true, nil
}

// releaseOld ends the old generation's records of the unmarked block at
// key as it leaves the run, by the sweep or a rebirth, and returns the block
// keys of the children outside the run its old list still holds, as
// oldListHolds decides, which lose that holder. It deletes the block's due
// cascade, which the release takes over, uncounting it in state, which the
// caller writes, and moves its kindUnmarkedCount record, the holders it
// gained and lost since the flip, over its frozen kindCount record. A child
// inside the run keeps no holder from it, since the flip dropped the
// holders of every unmarked block.
func (r *Records) releaseOld(ctx context.Context, state *LivenessState, key []byte) ([][]byte, error) {
	// Read whether the old list holds, and drop its cascade.
	holds, due, err := r.oldListHolds(ctx, key)
	if err != nil {
		return nil, err
	}
	if due {
		if err := r.deleteCascade(ctx, state, key); err != nil {
			return nil, err
		}
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
// lose that holder. The caller writes the new list and state.
func (r *Records) rebirth(ctx context.Context, state *LivenessState, ref *block.BlockRef) ([][]byte, error) {
	key := BlockKey(ref)
	released, err := r.releaseOld(ctx, state, key)
	if err != nil {
		return nil, err
	}
	return released, r.tx.Set(ctx, r.key(kindUnmarked, key), runReborn)
}

// outsideRun returns the block keys in packed that have no run entry.
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
func (c *Collector) settle(ctx context.Context, past func() bool, res *CollectResult) (bool, error) {
	// Read the generation.
	var gen Generation
	err := c.store.View(ctx, func(rec *Records) error {
		var err error
		gen, err = rec.generation(ctx)
		return err
	})
	if err != nil || gen == Generation_GENERATION_SETTLED {
		return false, err
	}

	// Sweep a flipped Volume: one pass from the start of the run that
	// reaches its end deletes every unmarked block, since none is added
	// while Sweeping, and then the run goes and the state settles.
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
// follows deletes it, so the front waits for one recount at most.
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
		holders, ripe, err := rec.GetHolders(ctx, ref)
		if err != nil {
			return from, false, err
		}
		if !listed || holders != 0 || ripe > cursor || cursor < state.GetHoldMark() {
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
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN {
		return err
	}
	if ok, err := rec.freeable(ctx, state); err != nil || !ok {
		return err
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

	// Flip.
	state.Generation = Generation_GENERATION_SWEEPING
	res.Flipped = true
	return rec.SetState(ctx, state)
}

// sweepSlice deletes each unmarked block of up to a slice of run entries
// after the key position from, while the generation is Sweeping and the
// Volume adopted: its bytes, list, proof and run entry, with one journal
// down pair for each child its old list still holds, as releaseOld decides,
// as a free's cascade releases them. Its count keeps the holders it gained
// since the flip, as a free keeps a block's netted holders. A reborn entry
// stays, since the sweep of an unmarked parent skips a reborn child, whose
// rebirth took back what the old list gave it. It reports whether it
// reached the run's end.
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
	keys, reborn, full, err := c.runEntries(ctx, rec, from, sliceFrees)
	if err != nil {
		return from, false, false, err
	}

	// Delete each unmarked block, and journal the release of its children.
	var down [][]byte
	for i, key := range keys {
		from = key
		if reborn[i] {
			continue
		}
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return from, false, false, err
		}
		children, err := rec.releaseOld(ctx, state, key)
		if err != nil {
			return from, false, false, err
		}
		down = append(down, children...)
		if err := blocks.RmBlock(ctx, ref); err != nil {
			return from, false, false, err
		}
		for _, del := range []func(context.Context, *block.BlockRef) error{rec.DeleteChildren, rec.DeleteProof} {
			if err := del(ctx, ref); err != nil {
				return from, false, false, err
			}
		}
		if err := rec.tx.Delete(ctx, rec.key(kindUnmarked, key)); err != nil {
			return from, false, false, err
		}
		res.Swept++
	}
	if len(down) != 0 {
		return from, full, !full, rec.appendDowns(ctx, state, c.opts.Now(), down)
	}
	return from, full, !full, rec.SetState(ctx, state)
}

// dropRunSlice deletes up to a slice of run entries after the key position
// from, once a sweep pass left no unmarked block, and, past the last,
// deletes the completion entry and settles the generation.
func (c *Collector) dropRunSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Drop only while Sweeping.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_SWEEPING {
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
	res.Settled = true
	return from, false, rec.SetState(ctx, state)
}

// runEntries returns the block keys of up to a slice of base run entries
// after the key position from, past the completion entry, whether each is
// reborn, and whether the slice was full.
func (c *Collector) runEntries(ctx context.Context, rec *Records, from []byte, base int) ([][]byte, []bool, bool, error) {
	// Scan past from, skipping the completion entry, until the slice fills.
	seek := from
	if from != nil {
		seek = append(bytes.Clone(from), 0)
	}
	budget := c.share(base)
	var keys [][]byte
	var reborn []bool
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
		reborn = append(reborn, bytes.Equal(v, runReborn))
		return true, err
	})
	return keys, reborn, full, err
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
