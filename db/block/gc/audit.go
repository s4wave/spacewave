package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// auditSampleSize bounds the blocks an audit lists by name for each finding.
const auditSampleSize = 8

// AuditResult compares what the named roots reach with what the collector
// would keep. The audit frees nothing; it records the holes it finds.
type AuditResult struct {
	// Mark is what the walk from the roots reached.
	Mark *Mark
	// Stored counts the stored blocks, which have a child list.
	Stored uint64
	// Leaked counts the stored blocks no root reaches that the collector has
	// not queued and the release in progress does not name. After a pass
	// that left nothing to do, that is garbage a hole, a put outside a batch
	// or a lost release keeps; during a cascade it also counts the children
	// of blocks still queued.
	Leaked uint64
	// LiveQueued counts the blocks a root reaches that sit on the free queue
	// with no journal record past the cursor adding a holder: blocks the
	// collector would free while they are live. Any is an error in the
	// counts.
	LiveQueued uint64
	// LiveQueuedSample names up to auditSampleSize of them.
	LiveQueuedSample []*block.BlockRef
	// Undercount counts the stored blocks whose netted holders, with the
	// journal past the cursor, are fewer than the roots, child lists and
	// release in progress that name them, such as a hole put again before an
	// audit recorded it. Each may be freed while a root still reaches it.
	Undercount uint64
	// UndercountSample names up to auditSampleSize of them.
	UndercountSample []*block.BlockRef
	// Holes counts the holes under a live root this audit recorded.
	Holes uint64
	// Unlisted counts the blocks whose bytes the Volume holds without a child
	// list that no root reaches: garbage nothing frees, since only a block
	// with a list is stored for the collector.
	Unlisted uint64
	// UnlistedSample names up to auditSampleSize of them.
	UnlistedSample []*block.BlockRef
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

		// Read what the journal past the cursor does to each block.
		tail, delta, err := journalTail(ctx, rec, state)
		if err != nil {
			return err
		}

		// Compare each stored block's liveness with the queue, and its count
		// once the Volume keeps counts.
		counted := tally
		if state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			counted = nil
		}
		releasing := newReleaseCursor(rec, state.GetRelease())
		err = rec.ScanChildren(ctx, func(ref *block.BlockRef, _ []*block.BlockRef) error {
			return res.checkStored(ctx, rec, ref, mark, counted, releasing, tail, delta)
		})
		if err != nil {
			return err
		}
		holes, err = holeCounts(ctx, rec, state, mark.AbsentRefs, tally, delta)
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
				if len(res.UnlistedSample) < auditSampleSize {
					res.UnlistedSample = append(res.UnlistedSample, ref)
				}
			}
			return nil
		})
	})
}

// checkStored compares one stored block with the walk, the queue and the
// holders that name it, taking the stored blocks in block key order. A block
// releasing names is no leak: the release in progress takes its freed
// parent's holder later.
func (res *AuditResult) checkStored(
	ctx context.Context,
	rec *Records,
	ref *block.BlockRef,
	mark *Mark,
	tally map[string]uint64,
	releasing *releaseCursor,
	tail map[string]struct{},
	delta map[string]int64,
) error {
	// Read its netted holders, whether it is queued, the tail and the
	// release.
	res.Stored++
	key := BlockKey(ref)
	holders, queued, err := nettedHolders(ctx, rec, ref)
	if err != nil {
		return err
	}
	_, held := tail[string(key)]
	pending, err := releasing.has(ctx, key)
	if err != nil {
		return err
	}

	// Count a live block the collector would free, and a leak.
	switch {
	case mark.Has(ref) && queued && !held:
		res.LiveQueued++
		if len(res.LiveQueuedSample) < auditSampleSize {
			res.LiveQueuedSample = append(res.LiveQueuedSample, ref)
		}
	case !mark.Has(ref) && !queued && !pending:
		res.Leaked++
	}

	// Count a block with fewer holders than name it, when tally is set.
	if tally == nil {
		return nil
	}
	if int64(holders)+delta[string(key)] < int64(tally[string(key)]) { //nolint:gosec // counts stay far below the int64 range.
		res.Undercount++
		if len(res.UndercountSample) < auditSampleSize {
			res.UndercountSample = append(res.UndercountSample, ref)
		}
	}
	return nil
}

// nettedHolders returns the holders the counts give ref, as the collector
// nets them: its count plus the implied holder, or none while it is queued.
func nettedHolders(ctx context.Context, rec *Records, ref *block.BlockRef) (uint64, bool, error) {
	queued, err := rec.Queued(ctx, ref)
	if err != nil || queued {
		return 0, queued, err
	}
	count, err := rec.GetCount(ctx, ref)
	return count + 1, false, err
}

// tallyHolders counts, for each named block, the roots, the child lists and
// the release in progress that name it, in rec. A child the release has not
// reached yet still holds the freed block's count, though the block's list is
// gone, so leaving it out of the tally would restore a hole's count one too
// low and free the hole while a root still reaches it.
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

	// Count the child lists.
	err = rec.ScanChildren(ctx, func(_ *block.BlockRef, children []*block.BlockRef) error {
		for _, child := range children {
			tally[string(BlockKey(child))]++
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Count the children the release in progress has not released.
	err = rec.scanReleasePending(ctx, state.GetRelease(), func(child *block.BlockRef) error {
		tally[string(BlockKey(child))]++
		return nil
	})
	return tally, err
}

// journalTail reads the journal past the cursor and the fill records not
// netted yet: the blocks their up pairs name, and each block's pairs the
// counts do not include yet, skipping the pairs of the record at the cursor
// its net offset already applied.
func journalTail(ctx context.Context, rec *Records, state *LivenessState) (map[string]struct{}, map[string]int64, error) {
	// Sum the journal records past the cursor.
	tail := make(map[string]struct{})
	delta := make(map[string]int64)
	err := rec.ScanJournal(ctx, state.GetCursor(), func(seq uint64, r *JournalRecord) (bool, error) {
		// Skip the pairs of the cursor's record that the counts include.
		skip := uint64(0)
		if seq == state.GetCursor() {
			skip = state.GetNetOffset()
		}

		// Add the record's up pairs, then its down pairs.
		var i uint64
		for _, up := range r.GetUp() {
			tail[string(up)] = struct{}{}
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
		return nil, nil, err
	}

	// Add the pairs of the fill records not netted yet, which the counts do
	// not include either.
	err = rec.scanFillRecords(ctx, func(fr fillRecord) (bool, error) {
		for _, up := range fr.rec.GetUp() {
			tail[string(up)] = struct{}{}
			delta[string(up)]++
		}
		for _, down := range fr.rec.GetDown() {
			delta[string(down)]--
		}
		return true, nil
	})
	return tail, delta, err
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
// snapshot only by netting a journal or fill record and by a release taking a
// freed parent's holder, each by the change to its true holders, or by a
// clamp at zero, which leaves them higher. Land names a hole only through an
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
		key := string(BlockKey(ref))
		named := tally[key]
		if named == 0 {
			continue
		}

		// Before the collector nets, keep the tally as the least holders.
		if !adopted {
			holes = append(holes, holeCount{ref: ref, floor: named})
			continue
		}

		// Otherwise find the holders the counts lack.
		holders, _, err := nettedHolders(ctx, rec, ref)
		if err != nil {
			return nil, err
		}
		missing := int64(named) - int64(holders) - delta[key] //nolint:gosec // counts stay far below the int64 range.
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
	for len(holes) != 0 {
		// Take the next slice.
		n := min(len(holes), sliceRecords)
		slice := holes[:n]
		holes = holes[n:]

		// Record each hole the store still lacks.
		err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
			for _, h := range slice {
				if err := recordHole(ctx, rec, h, res); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Let a waiting writer in.
		if err := context.Cause(ctx); err != nil {
			return err
		}
	}
	return nil
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
	hole, err := rec.IsHole(ctx, h.ref)
	if err != nil || hole {
		return err
	}

	// Record it.
	if err := rec.SetHole(ctx, h.ref); err != nil {
		return err
	}
	res.Holes++

	// Raise its netted holders, taking it off the queue, where it holds
	// none.
	holders, queued, err := nettedHolders(ctx, rec, h.ref)
	if err != nil {
		return err
	}
	next := max(holders+h.add, h.floor)
	if next == holders {
		return nil
	}
	if queued {
		if err := rec.Dequeue(ctx, h.ref); err != nil {
			return err
		}
	}
	return rec.SetCount(ctx, h.ref, next-1)
}
