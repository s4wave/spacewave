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
//   - Nothing is deleted: the free parks each block it would free, and
//     enters it in the run, as Records.park describes.
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
// Before the flip the collector seeds the front, in slices: it pushes every
// unverified block a named root or a down pair past the cursor names, as
// seed describes, the model's WalkStart. The flip is one state write, to
// Sweeping, in a transaction that finds the Volume adopted with every
// cascade and fill up netted, no list rewrite awaiting a recount, no
// recorded hole, the run complete, the front empty and every ripe Dead
// block parked: the model's WalkEnd. Every block still unverified is then
// unreachable: no seed names it, and no young or verified block does, since
// the front is empty. The run names those blocks and the parked ones.
//
// While Sweeping, an empty run entry names an unmarked block,
// which every site that asks whether a block is stored answers absent, as
// Unmarked describes. The flip, which requires no cascade due, freezes each
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
// Sweeping and the block's run entry is neither reborn nor swept. It reads
// nothing more than the generation outside Sweeping.
func (r *Records) Unmarked(ctx context.Context, key []byte) (bool, error) {
	gen, err := r.generation(ctx)
	if err != nil || gen != Generation_GENERATION_SWEEPING {
		return false, err
	}
	value, found, err := r.tx.Get(ctx, r.key(kindUnmarked, key))
	return found && len(value) == 0, err
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
// which it reads as unlisted. Under HoldUntilFree a stored block holds its
// children until its free whatever its count, and the free skips an
// unmarked block, so its old list always holds them.
func (r *Records) oldListHolds(ctx context.Context, state *LivenessState, key []byte) (bool, error) {
	// Under HoldUntilFree a stored block's list holds until its free.
	if state.GetHoldRule() == HoldRule_HOLD_RULE_HOLD_UNTIL_FREE {
		return true, nil
	}

	// Read the count the flip froze; a block with none is unmarked.
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
func (r *Records) releaseOld(ctx context.Context, state *LivenessState, key []byte) ([][]byte, error) {
	// Read whether the old list holds.
	holds, err := r.oldListHolds(ctx, state, key)
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
// front, and the run unless the free parked it, as Records.park describes,
// and its unverified children join the front, which keeps the front
// invariant. It reads the block's count and parked key.
func (r *Records) markedYoung(ctx context.Context, key, packed []byte, skip func(key []byte) bool) error {
	// Leave the run unless parked.
	parked, err := r.parked(ctx, key)
	if err != nil {
		return err
	}
	if !parked {
		if err := r.tx.Delete(ctx, r.key(kindUnmarked, key)); err != nil {
			return err
		}
	}

	// Leave the front and push the children.
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
func (r *Records) rebirth(ctx context.Context, state *LivenessState, ref *block.BlockRef) ([][]byte, error) {
	key := BlockKey(ref)
	released, err := r.releaseOld(ctx, state, key)
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

	// Build the run and seed the front, each resuming where the last pass
	// stopped.
	more, err := resumeSlices(ctx, c.update, &c.runFrom, past, func(rec *Records, from []byte) ([]byte, bool, error) {
		return c.runSlice(ctx, rec, res, from)
	})
	if err != nil || more {
		return more, err
	}
	if more, err := c.seed(ctx, past, res); err != nil || more {
		return more, err
	}

	// Take the free-proven entries off the front, then try the flip.
	more, err = eachSlice(ctx, c.update, []byte(nil), past, func(rec *Records, blocks block.StoreOps, from []byte) ([]byte, bool, error) {
		return c.frontSlice(ctx, rec, blocks, res, from)
	})
	if err != nil || more {
		return more, err
	}
	return false, c.update(ctx, func(rec *Records, _ block.StoreOps) error {
		return c.flip(ctx, rec, res)
	})
}

// seed pushes onto the front, once per collector while Open, each unverified
// block a named root names and each block a down pair past the cursor
// names, which a reader or writer may still hold: the named roots in slices,
// then the journal records past the cursor in slices, resuming where the
// last pass stopped. It reports whether it stopped at past with work left.
//
// Invariant: once the seeding is done, every unverified block such a seed
// would name is on the front or was taken off it by the walk or the free
// proof. A root a Land moves onto a block after its slice passed the root
// pushes the block, as holdStored does while Open; a down pair it writes
// names the block the root held, which a slice passed or a Land pushed; and
// a record the cursor passes leaves the seeds. So the flip needs no seeding
// of its own, only the empty front.
//
// Cost: one sequential read of the named roots and of the journal records
// past the cursor, and one list read per root and per down pair, once per
// collector; each slice holds the writer for at most sliceRecords roots or
// records.
func (c *Collector) seed(ctx context.Context, past func() bool, res *CollectResult) (bool, error) {
	// Seed the named roots first.
	if c.seeded {
		return false, nil
	}
	if !c.seededRoots {
		more, err := resumeSlices(ctx, c.update, &c.seedRoot, past, func(rec *Records, from []byte) ([]byte, bool, error) {
			return c.seedRootSlice(ctx, rec, res, from)
		})
		if err != nil || more {
			return more, err
		}
		c.seededRoots = true
	}

	// Then the journal records past the cursor.
	more, err := resumeSlices(ctx, c.update, &c.seedSeq, past, func(rec *Records, from uint64) (uint64, bool, error) {
		return c.seedJournalSlice(ctx, rec, res, from)
	})
	c.seeded = err == nil && !more
	return more, err
}

// resumeSlices runs slice as eachSlice does, from the position at pos, and
// on success stores at pos the position the last committed slice returned,
// so a later call resumes there. It reports whether it stopped at past with
// work left.
func resumeSlices[P any](
	ctx context.Context,
	update func(context.Context, func(*Records, block.StoreOps) error) error,
	pos *P,
	past func() bool,
	slice func(rec *Records, from P) (P, bool, error),
) (bool, error) {
	reached := *pos
	more, err := eachSlice(ctx, update, *pos, past, func(rec *Records, _ block.StoreOps, from P) (P, bool, error) {
		next, more, err := slice(rec, from)
		reached = next
		return next, more, err
	})
	if err == nil {
		*pos = reached
	}
	return more, err
}

// seedRootSlice pushes the block of each of up to a slice of named roots
// after the root key from onto the front when its list is unverified, while
// the generation is Open, and returns the last root key it read and whether
// more may follow.
func (c *Collector) seedRootSlice(ctx context.Context, rec *Records, res *CollectResult, from []byte) ([]byte, bool, error) {
	// Read a slice of roots while Open.
	gen, err := rec.generation(ctx)
	if err != nil || gen != Generation_GENERATION_OPEN {
		return from, false, err
	}
	budget := c.share(sliceRecords)
	roots, err := rec.scanAfter(ctx, kindRoot, from, budget)
	if err != nil {
		return from, false, err
	}

	// Push each root's block when its list is unverified.
	for _, root := range roots {
		from = root.key
		entry := &RootEntry{}
		if err := entry.UnmarshalVT(root.value); err != nil {
			return from, false, err
		}
		pushed, err := rec.pushUnverified(ctx, entry.GetRef())
		if err != nil {
			return from, false, err
		}
		if pushed {
			res.Seeded++
		}
	}
	return from, len(roots) == budget, nil
}

// seedJournalSlice pushes onto the front each unverified block a down pair
// of up to a slice of journal records names, from the sequence from or the
// cursor, whichever is later, while the generation is Open, and returns the
// sequence after the last record it read and whether more may follow.
func (c *Collector) seedJournalSlice(ctx context.Context, rec *Records, res *CollectResult, from uint64) (uint64, bool, error) {
	// Start at the cursor or later, while Open.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN {
		return from, false, err
	}
	from = max(from, state.GetCursor())

	// Push the unverified blocks each record's down pairs name.
	budget, n := c.share(sliceRecords), 0
	err = rec.ScanJournal(ctx, from, func(seq uint64, jr *JournalRecord) (bool, error) {
		// Stop at the budget.
		if n == budget {
			return false, nil
		}
		n++
		from = seq + 1

		// Push each down pair's block.
		for _, key := range jr.GetDown() {
			pushed, err := rec.pushUnverified(ctx, key)
			if err != nil {
				return false, err
			}
			if pushed {
				res.Seeded++
			}
		}
		return true, nil
	})
	return from, n == budget, err
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
// that recount, which returns it to the front, as returnHeld describes. A
// block a journal record past the cursor names stays, as the free keeps it
// (T1): the pop reads the tail index, synced to the journal's end in the
// slice's transaction, as the free does. A slice that pops raises the pop
// mark to the latest ripe sequence it popped, as kindPopMark describes, at
// the cost of one key read and write per such slice. A list lasts from the
// block's put to its free, and the free parks while Open, so an entry the
// store does not list is damage that lost the list. While the bytes remain
// it stays, for the walk decodes them and writes the list again; once they
// are gone too nothing can decode the block, which names no child the walk
// could still find, and it pops under the same proof, so damage cannot keep
// the generation Open.
func (c *Collector) frontSlice(ctx context.Context, rec *Records, blocks block.StoreOps, res *CollectResult, from []byte) ([]byte, bool, error) {
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
	reached, err := c.tail.sync(ctx, rec, state, c.share(slicePairs), c.waiting)
	if err != nil {
		return from, false, err
	}
	if !reached {
		res.TailReading++
		return from, true, nil
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
	var popped, stalled bool
	var poppedRipe uint64
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
		if holders != 0 || ripe > cursor || c.tail.has(key) {
			continue
		}
		if !listed {
			stored, err := blocks.GetBlockExists(ctx, ref)
			if err != nil {
				return from, false, err
			}
			if stored {
				continue
			}
		}
		if cursor < state.GetHoldMark() {
			stalled = true
			continue
		}
		if err := rec.tx.Delete(ctx, rec.key(kindFront, key)); err != nil {
			return from, false, err
		}
		popped, poppedRipe = true, max(poppedRipe, ripe)
		res.Popped++
	}

	// Report a wait on the hold mark, and raise the pop mark past what this
	// slice popped.
	if stalled {
		res.StallHoldMark++
	}
	if !popped {
		return from, full, nil
	}
	return from, full, rec.raisePopMark(ctx, poppedRipe)
}

// flip moves an Open Volume to Sweeping when the flip's checks pass, as the
// settle describes, or, when a list rewrite awaits a recount, returns the
// Volume to unadopted. The settle calls it only once seed is done, so the
// empty front covers every seed.
//
// Cost: point reads of the run's completion entry and the pop mark and up
// to four prefix seeks, in one transaction, once per collector pass until
// the flip.
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

	// Require the adoption's marker netted: a head a reader saw before the
	// mark, which the walk does not seed, is unmarked only once it is Grace
	// old. The mark is fixed, so no write can hold the flip here.
	if state.GetCursor() < state.GetAdoptMark() {
		return nil
	}

	// A rewritten list's new children lack their holders: recount first.
	// This is the settle's one return to unadopted, and it runs only while
	// Open. A readopt while Sweeping, such as a change of the hold rule,
	// clears both count records of each unmarked block and counts every
	// stored list again: SetHolders writes an unmarked block's count to
	// kindUnmarkedCount, and with no kindCount record oldListHolds reports
	// that its old list holds the children the recount gave it.
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

	// Require every Dead block at or below the pop mark parked, and so in
	// the run. A marked block that names a child the front popped is Dead at
	// a ripe sequence no later than the child's; left present, a hold onto
	// it would revive its list over children the sweep deletes. A queued
	// block above the mark names no popped child, so a writer that keeps
	// the hold mark ahead of the cursor, which keeps the free from parking,
	// cannot hold the flip.
	mark, popped, err := rec.popMark(ctx)
	if err != nil {
		return err
	}
	if popped {
		ripe, err := rec.ripeQueued(ctx, min(mark, state.GetCursor()))
		if err != nil || ripe {
			return err
		}
	}

	// Flip.
	state.Generation = Generation_GENERATION_SWEEPING
	res.Flipped = true
	if err := rec.tx.Delete(ctx, rec.key(kindPopMark)); err != nil {
		return err
	}
	return rec.SetState(ctx, state)
}

// sweepSlice deletes each unmarked block of up to a slice of run entries
// after the key position from, while the generation is Sweeping and the
// Volume adopted: its bytes, list and proof, with one journal down pair for
// each child its old list still holds, as releaseOld decides, as a free's
// cascade releases them, and marks its run entry swept. Its count keeps the
// holders it gained since the flip, as a free keeps a block's netted
// holders. It skips a reborn or swept entry. A child inside the run at the
// flip keeps no holder from the old list, so the sweep of an unmarked
// parent releases nothing to it, even after a put stored it again. It
// reports whether it reached the run's end.
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
		if err := rec.tx.Set(ctx, rec.key(kindUnmarked, key), runSwept); err != nil {
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
