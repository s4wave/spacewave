package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// AuditResult compares what the named roots reach with what the collector
// would keep. The audit frees nothing; it records the holes it finds.
type AuditResult struct {
	// Mark is what the walk from the roots reached.
	Mark *Mark
	// Stored counts the stored blocks, which have a child list.
	Stored uint64
	// Leaked counts the stored blocks no root reaches that have holders and
	// that the paged release in progress does not name. After a pass that
	// left nothing to do, that is garbage a hole, a put outside a batch or a
	// lost release keeps; while a cascade is due it also counts the children
	// of blocks still Dying, and while the journal past the cursor holds
	// downs, the blocks they release.
	Leaked uint64
	// LiveQueued counts the blocks a root reaches that the collector would
	// free now: Dead, ripe, with no cascade due, no fill up unapplied and the
	// cursor past the hold mark. Any is an error in the counts.
	LiveQueued uint64
	// LiveQueuedSample names up to sampleSize of them.
	LiveQueuedSample []*block.BlockRef
	// Undercount counts the stored blocks whose netted holders, with the
	// journal past the cursor, are fewer than the roots, child lists and
	// release in progress that name them, such as a hole put again before an
	// audit recorded it. Each may be freed while a root still reaches it.
	Undercount uint64
	// UndercountSample names up to sampleSize of them.
	UndercountSample []*block.BlockRef
	// Holes counts the holes under a live root this audit recorded.
	Holes uint64
	// Unlisted counts the blocks whose bytes the Volume holds without a child
	// list that no root reaches: garbage nothing frees, since only a block
	// with a list is stored for the collector.
	Unlisted uint64
	// UnlistedSample names up to sampleSize of them.
	UnlistedSample []*block.BlockRef
	// CascadeMiscount is the cascade records less the state's count of them,
	// while the Volume keeps the count. Any is an error in the counts: a
	// count below its records lets a free run while a cascade is due.
	CascadeMiscount int64
	// UnlistedCascades counts the cascade records due on blocks without a
	// child list, which no cascade can run. The next pass drops a down one;
	// an up one, a Holding block the store lost, keeps every free waiting
	// until a put refills the block.
	UnlistedCascades uint64
}

// Audit walks from the named roots of the Volume in one snapshot and compares
// the walk with the counts and the free queue, as AuditResult describes. It
// then records each hole it reached that no record names yet, restoring its
// count from the holders the snapshot gave it, so a later put of the hole
// counts it as stored, as Land describes. It keeps the reached set and a
// holder count per named block in memory, so it suits a copy host.
func Audit(ctx context.Context, store AdoptStore) (AuditResult, error) {
	// Read the snapshot: the walk, the holders, the stored blocks and what
	// each hole's count lacks.
	var res AuditResult
	var holes []holeCount
	err := store.View(ctx, func(rec *Records) error {
		// Walk from the roots, and count each block's holders.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		mark, err := Reachable(ctx, rec, store.BlockStored)
		if err != nil {
			return err
		}
		res.Mark = mark
		tally, err := tallyHolders(ctx, rec, state)
		if err != nil {
			return err
		}

		// Read what the journal past the cursor does to each block, and
		// whether the collector would free now.
		delta, err := journalTail(ctx, rec, state)
		if err != nil {
			return err
		}
		frees, err := rec.freeable(ctx, state)
		if err != nil {
			return err
		}
		frees = frees && state.GetCursor() >= state.GetHoldMark()

		// Compare each stored block's liveness with the queue, and its count
		// once the Volume keeps counts.
		counted := tally
		if state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			counted = nil
		}
		releasing := newReleaseCursor(rec, state.GetRelease())
		err = rec.ScanChildren(ctx, func(ref *block.BlockRef, _ []*block.BlockRef) error {
			return res.checkStored(ctx, rec, state, ref, mark, counted, releasing, frees, delta)
		})
		if err != nil {
			return err
		}
		holes, err = holeCounts(ctx, rec, state, mark.AbsentRefs, tally, delta)
		if err != nil || !cascadesCounted(state) {
			return err
		}

		// Compare the cascade records with their count, and count those on
		// blocks without a list.
		var due int64
		err = rec.scanCascades(ctx, nil, func(key []byte, _ byte) (bool, error) {
			due++
			listed, err := rec.tx.Exists(ctx, rec.key(kindChildren, key))
			if !listed {
				res.UnlistedCascades++
			}
			return true, err
		})
		res.CascadeMiscount = due - int64(state.GetCascades()) //nolint:gosec // counts stay far below the int64 range.
		return err
	})
	if err != nil {
		return res, err
	}

	// Count the unreached bytes stored without a list, then record the
	// holes under a live root.
	if err := res.countUnlisted(ctx, store); err != nil {
		return res, err
	}
	return res, recordHoles(ctx, store, holes, &res)
}

// countUnlisted counts the blocks whose bytes the store holds without a child
// list that the walk did not reach, checking each run's lists in one
// snapshot. A block a put lists after the run's snapshot counts too.
func (res *AuditResult) countUnlisted(ctx context.Context, store AdoptStore) error {
	return store.ScanBlocks(ctx, func(refs []*block.BlockRef) error {
		return store.View(ctx, func(rec *Records) error {
			for _, ref := range refs {
				if res.Mark.Has(ref) {
					continue
				}
				listed, err := rec.HasChildren(ctx, ref)
				if err != nil || listed {
					if err != nil {
						return err
					}
					continue
				}
				res.Unlisted++
				res.UnlistedSample = sample(res.UnlistedSample, ref)
			}
			return nil
		})
	})
}

// checkStored compares one stored block with the walk, the queue and the
// holders that name it, taking the stored blocks in block key order. A block
// a paged release names is no leak: the release takes its parent's holder
// later.
func (res *AuditResult) checkStored(
	ctx context.Context,
	rec *Records,
	state *LivenessState,
	ref *block.BlockRef,
	mark *Mark,
	tally map[string]uint64,
	releasing *releaseCursor,
	frees bool,
	delta map[string]int64,
) error {
	// Skip an unmarked block, which the sweep deletes: it is absent from
	// the flip on, and its holders since then are checked once it leaves
	// the run. Then read its netted holders and whether the release in
	// progress names it.
	res.Stored++
	key := BlockKey(ref)
	if unmarked, err := rec.Unmarked(ctx, key); err != nil || unmarked {
		return err
	}
	holders, ripe, err := rec.GetHolders(ctx, ref)
	if err != nil {
		return err
	}
	pending, err := releasing.has(ctx, key)
	if err != nil {
		return err
	}
	pending = pending && !state.GetRelease().GetRehold()

	// Count a live block the collector would free, and a leak.
	switch {
	case mark.Has(ref) && holders == 0 && frees && ripe <= state.GetCursor():
		res.LiveQueued++
		res.LiveQueuedSample = sample(res.LiveQueuedSample, ref)
	case !mark.Has(ref) && holders != 0 && !pending:
		res.Leaked++
	}

	// Count a block with fewer holders than name it, when tally is set.
	if tally == nil {
		return nil
	}
	if int64(holders)+delta[string(key)] < int64(tally[string(key)]) { //nolint:gosec // counts stay far below the int64 range.
		res.Undercount++
		res.UndercountSample = sample(res.UndercountSample, ref)
	}
	return nil
}

// counts reports whether the children of the stored block ref count it as a
// holder in an adopted Volume: it is Live or Dying, its rel flag clear. An
// unmarked block's old list counts as the flip froze it, as oldListHolds
// reads.
func counts(ctx context.Context, rec *Records, ref *block.BlockRef) (bool, error) {
	// Read an unmarked block's old list as frozen.
	key := BlockKey(ref)
	unmarked, err := rec.Unmarked(ctx, key)
	if err != nil {
		return false, err
	}
	if unmarked {
		holds, err := rec.oldListHolds(ctx, key)
		return holds, err
	}

	// Otherwise compare its holders with its due cascade.
	holders, _, err := rec.GetHolders(ctx, ref)
	if err != nil {
		return false, err
	}
	_, due, err := rec.getCascade(ctx, key)
	return (holders == 0) == due, err
}

// tallyHolders counts, for each named block, the roots, the child lists and
// the paged cascade in progress that name it, in rec. Once the Volume is
// adopted, a block's list counts only while its children count it, Live or
// Dying; a child a paged release has not reached yet still counts its
// parent, and one a paged re-hold has not reached does not yet. Leaving the
// release's children out would restore a hole's count one too low and free
// the hole while a root still reaches it.
func tallyHolders(ctx context.Context, rec *Records, state *LivenessState) (map[string]uint64, error) {
	// Count the roots.
	tally := make(map[string]uint64)
	err := rec.ScanRoots(ctx, func(_, _ string, entry *RootEntry) error {
		tally[string(entry.GetRef())]++
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Count the child lists of the blocks their children count.
	adopted := state.GetAdoption() == Adoption_ADOPTION_ADOPTED
	err = rec.ScanChildren(ctx, func(ref *block.BlockRef, children []*block.BlockRef) error {
		if adopted {
			held, err := counts(ctx, rec, ref)
			if err != nil || !held {
				return err
			}
		}
		for _, child := range children {
			tally[string(BlockKey(child))]++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Count the children a paged release has not moved, and take back the
	// ones a paged re-hold has not.
	rehold := state.GetRelease().GetRehold()
	err = rec.scanReleasePending(ctx, state.GetRelease(), func(child *block.BlockRef) error {
		key := string(BlockKey(child))
		switch {
		case !rehold:
			tally[key]++
		case tally[key] != 0:
			tally[key]--
		}
		return nil
	})
	return tally, err
}

// journalTail reads the journal past the cursor and the fill records not
// netted yet: each block's pairs the counts do not include yet, skipping the
// entries of the record at the cursor its net offset already applied. A
// record's again list moves no holder.
func journalTail(ctx context.Context, rec *Records, state *LivenessState) (map[string]int64, error) {
	// Sum the journal records past the cursor.
	delta := make(map[string]int64)
	err := rec.ScanJournal(ctx, state.GetCursor(), func(seq uint64, r *JournalRecord) (bool, error) {
		// Skip the entries of the cursor's record that the counts include.
		skip := uint64(0)
		if seq == state.GetCursor() {
			skip = state.GetNetOffset()
		}

		// Add the record's up pairs, then its down pairs.
		var i uint64
		for _, up := range r.GetUp() {
			if i >= skip {
				delta[string(up)]++
			}
			i++
		}
		for _, down := range r.GetDown() {
			if i >= skip {
				delta[string(down)]--
			}
			i++
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}

	// Add the fill ups not applied yet and the downs of the fill records not
	// netted yet, which the counts do not include either.
	err = rec.scanFillUps(ctx, func(_ []byte, children []*block.BlockRef) (bool, error) {
		for _, child := range children {
			delta[string(BlockKey(child))]++
		}
		return true, nil
	})
	if err != nil {
		return nil, err
	}
	err = rec.scanFillRecords(ctx, 0, func(fr fillRecord) (bool, error) {
		for _, down := range fr.rec.GetDown() {
			delta[string(down)]--
		}
		return true, nil
	})
	return delta, err
}

// holeCount is what an audit's snapshot gives the count of one hole.
type holeCount struct {
	// ref is the hole.
	ref *block.BlockRef
	// add is the holders the snapshot tallied beyond the hole's netted
	// holders and the journal past the cursor, none when those hold as many.
	add uint64
	// floor is the least netted holders the hole keeps, in a Volume the
	// collector does not net yet, whose counts change only by an adoption.
	floor uint64
}

// holeCounts returns, from the snapshot in rec, what each hole in absent that
// a holder names lacks in its count, using the tally and journal delta of the
// same snapshot.
//
// Invariant: in an adopted Volume, a hole's netted holders change after the
// snapshot only by netting a journal or fill record, a fill up or a cascade,
// each by the change to its true holders, or by an underflow's floor at one,
// which leaves them higher. Land names a hole only through an
// up pair in the journal. So the holders the snapshot found missing are still
// missing when the hole is recorded, and adding them to its netted holders
// then counts every holder once, including one a Land gave the hole after
// the snapshot, which the counts or the journal past the cursor carry and the
// tally lacks.
func holeCounts(
	ctx context.Context,
	rec *Records,
	state *LivenessState,
	absent []*block.BlockRef,
	tally map[string]uint64,
	delta map[string]int64,
) ([]holeCount, error) {
	adopted := state.GetAdoption() == Adoption_ADOPTION_ADOPTED
	holes := make([]holeCount, 0, len(absent))
	for _, ref := range absent {
		// Skip a block nothing names.
		key := BlockKey(ref)
		named := tally[string(key)]
		if named == 0 {
			continue
		}

		// Before the collector nets, keep the tally as the least holders.
		if !adopted {
			holes = append(holes, holeCount{ref: ref, floor: named})
			continue
		}

		// Otherwise find the holders the counts lack.
		holders, _, err := rec.GetHolders(ctx, ref)
		if err != nil {
			return nil, err
		}
		missing := int64(named) - int64(holders) - delta[string(key)] //nolint:gosec // counts stay far below the int64 range.
		h := holeCount{ref: ref}
		if missing > 0 {
			h.add = uint64(missing)
		}
		holes = append(holes, h)
	}
	return holes, nil
}

// recordHoles records each hole in holes, sliceRecords per transaction,
// adding the holders its count lacks, as holeCounts describes: one netted
// holder stands for the parents a later put counts the hole as stored under,
// and the count holds the rest. It skips a block that a put stored, or an
// audit recorded, since the snapshot, and never lowers a count.
func recordHoles(ctx context.Context, store Store, holes []holeCount, res *AuditResult) error {
	_, err := eachSlice(ctx, store.Update, holes, nil, func(rec *Records, _ block.StoreOps, holes []holeCount) ([]holeCount, bool, error) {
		n := min(len(holes), sliceRecords)
		for _, h := range holes[:n] {
			if err := recordHole(ctx, rec, h, res); err != nil {
				return nil, false, err
			}
		}
		return holes[n:], n != len(holes), nil
	})
	return err
}

// recordHole records h as a hole and raises its netted holders by the
// holders its count lacks, or to its floor, unless a put stored it or an
// audit recorded it since the snapshot. It never lowers a count, so a hole
// whose counts hold more than the snapshot found keeps them.
func recordHole(ctx context.Context, rec *Records, h holeCount, res *AuditResult) error {
	// Skip a stored block and a recorded hole.
	listed, err := rec.HasChildren(ctx, h.ref)
	if err != nil || listed {
		return err
	}
	hole, err := rec.tx.Exists(ctx, rec.key(kindHole, BlockKey(h.ref)))
	if err != nil || hole {
		return err
	}

	// Record it.
	if err := rec.tx.Set(ctx, rec.key(kindHole, BlockKey(h.ref)), nil); err != nil {
		return err
	}
	res.Holes++

	// Raise its netted holders.
	holders, ripe, err := rec.GetHolders(ctx, h.ref)
	if err != nil {
		return err
	}
	next := max(holders+h.add, h.floor)
	if next == holders {
		return nil
	}

	// A hole at zero holders leaves the queue, or its due cascade, and is
	// Live. A refill writes its list back without giving its children a
	// holder, which is right for a hole a Live or Dying block left, whose
	// children still count it. A Dead block's children no longer count it,
	// so a refill of a Dead hole leaves them one holder short; a Dead block
	// loses its list only to a free, and a root reaching a freed block is
	// the error LiveQueued reports.
	if holders == 0 {
		key := BlockKey(h.ref)
		_, dying, err := rec.getCascade(ctx, key)
		if err != nil {
			return err
		}
		if dying {
			err = uncountCascade(ctx, rec, key)
		} else {
			err = rec.tx.Delete(ctx, rec.queueKey(ripe, key))
		}
		if err != nil {
			return err
		}
	}
	return rec.SetHolders(ctx, h.ref, next, 0)
}

// uncountCascade deletes the cascade record of the block at key, which has
// one, and uncounts it in the state.
func uncountCascade(ctx context.Context, rec *Records, key []byte) error {
	state, err := rec.GetState(ctx)
	if err != nil {
		return err
	}
	if err := rec.deleteCascade(ctx, state, key); err != nil {
		return err
	}
	return rec.SetState(ctx, state)
}

// AuditFront checks what kindFront documents while the generation is Open on
// an adopted Volume: each stored block with an unverified list that a young
// or verified list names is on the front, unless it has no holder, as the
// free proof pops it. The free proof and the flip rely on it: the walk
// decodes only the front, so a block missing from it while a marked list
// names it and something holds it is never verified, and the sweep deletes
// it. It returns how many times a marked list names a block that breaks it,
// and at most sampleSize of those blocks; outside Open or before the adoption ends it checks nothing. It
// walks every child list and reads each child's records, so it suits a test
// or a copy host.
func AuditFront(ctx context.Context, rec *Records) (missing uint64, sample []*block.BlockRef, err error) {
	// Check only an adopted Volume while Open.
	state, err := rec.GetState(ctx)
	if err != nil || state.GetGeneration() != Generation_GENERATION_OPEN || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
		return 0, nil, err
	}

	// Check each child of each marked list.
	err = rec.scan(ctx, rec.key(kindChildren), nil, func(_ []byte, value func() ([]byte, error)) (bool, error) {
		// Decode a marked list.
		v, err := value()
		if err != nil || !listMarked(v) {
			return err == nil, err
		}
		children, err := DecodeChildren(v)
		if err != nil {
			return false, err
		}

		// Count each held unverified child off the front.
		for _, child := range children {
			key := BlockKey(child)
			listed, marked, err := rec.listState(ctx, key)
			if err != nil {
				return false, err
			}
			if !listed || marked {
				continue
			}
			front, err := rec.tx.Exists(ctx, rec.key(kindFront, key))
			if err != nil {
				return false, err
			}
			if front {
				continue
			}
			holders, _, err := rec.GetHolders(ctx, child)
			if err != nil {
				return false, err
			}
			if holders == 0 {
				continue
			}
			missing++
			if len(sample) < sampleSize {
				sample = append(sample, child)
			}
		}
		return true, nil
	})
	return missing, sample, err
}
