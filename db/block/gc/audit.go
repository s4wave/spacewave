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
	// not queued. After a pass that left nothing to do, that is garbage a
	// hole, a put outside a batch or a lost release keeps; during a cascade
	// it also counts the children of blocks still queued.
	Leaked uint64
	// LiveQueued counts the blocks a root reaches that sit on the free queue
	// with no journal record past the cursor adding a holder: blocks the
	// collector would free while they are live. Any is an error in the
	// counts.
	LiveQueued uint64
	// LiveQueuedSample names up to auditSampleSize of them.
	LiveQueuedSample []*block.BlockRef
	// Undercount counts the stored blocks whose netted holders, with the
	// journal past the cursor, are fewer than the roots and child lists that
	// name them, such as a hole put again before an audit recorded it. Each
	// may be freed while a root still reaches it.
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
	// Read the snapshot: the walk, the holders and the stored blocks.
	var res AuditResult
	var tally map[string]uint64
	err := store.View(ctx, func(rec *Records) error {
		// Walk from the roots, and count each block's holders.
		mark, err := Reachable(ctx, rec, store.BlockStored)
		if err != nil {
			return err
		}
		res.Mark = mark
		if tally, err = tallyHolders(ctx, rec); err != nil {
			return err
		}

		// Read what the journal past the cursor does to each block.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
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
		return rec.ScanChildren(ctx, func(ref *block.BlockRef, _ []*block.BlockRef) error {
			return res.checkStored(ctx, rec, ref, mark, counted, tail, delta)
		})
	})
	if err != nil {
		return res, err
	}

	// Count the unreached bytes stored without a list, then record the
	// holes under a live root.
	if err := res.countUnlisted(ctx, store); err != nil {
		return res, err
	}
	return res, recordHoles(ctx, store, res.Mark.AbsentRefs, tally, &res)
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
// holders that name it.
func (res *AuditResult) checkStored(
	ctx context.Context,
	rec *Records,
	ref *block.BlockRef,
	mark *Mark,
	tally map[string]uint64,
	tail map[string]struct{},
	delta map[string]int64,
) error {
	// Read whether the block is queued, its count and the tail.
	res.Stored++
	key := BlockKey(ref)
	queued, err := rec.Queued(ctx, ref)
	if err != nil {
		return err
	}
	count, err := rec.GetCount(ctx, ref)
	if err != nil {
		return err
	}
	_, held := tail[string(key)]

	// Count a live block the collector would free, and a leak.
	switch {
	case mark.Has(ref) && queued && !held:
		res.LiveQueued++
		if len(res.LiveQueuedSample) < auditSampleSize {
			res.LiveQueuedSample = append(res.LiveQueuedSample, ref)
		}
	case !mark.Has(ref) && !queued:
		res.Leaked++
	}

	// Count a block with fewer holders than name it, when tally is set.
	if tally == nil {
		return nil
	}
	holders := int64(count) + 1 //nolint:gosec // counts stay far below the int64 range.
	if queued {
		holders = 0
	}
	if holders+delta[string(key)] < int64(tally[string(key)]) { //nolint:gosec // tallies stay far below the int64 range.
		res.Undercount++
		if len(res.UndercountSample) < auditSampleSize {
			res.UndercountSample = append(res.UndercountSample, ref)
		}
	}
	return nil
}

// tallyHolders counts, for each named block, the roots and the child lists
// that name it, in rec.
func tallyHolders(ctx context.Context, rec *Records) (map[string]uint64, error) {
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
	return tally, err
}

// journalTail reads the journal past the cursor: the blocks its up pairs
// name, and each block's pairs the counts do not include yet, skipping the
// pairs of the record at the cursor its net offset already applied.
func journalTail(ctx context.Context, rec *Records, state *LivenessState) (map[string]struct{}, map[string]int64, error) {
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
	return tail, delta, err
}

// recordHoles records each hole in absent that a live block's list names,
// sliceRecords per transaction, and restores its count from the holders tally
// gave it: one stands for the parents a later put counts the hole as stored
// under, and the count holds the rest. It skips a block that a put stored, or
// an audit recorded, since the snapshot, and never lowers a count.
func recordHoles(ctx context.Context, store Store, absent []*block.BlockRef, tally map[string]uint64, res *AuditResult) error {
	for len(absent) != 0 {
		// Take the next slice.
		n := min(len(absent), sliceRecords)
		slice := absent[:n]
		absent = absent[n:]

		// Record each hole the store still lacks.
		err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
			for _, ref := range slice {
				if err := recordHole(ctx, rec, ref, tally[string(BlockKey(ref))], res); err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return err
		}

		// Let a waiting writer in.
		if err := yieldWriters(ctx); err != nil {
			return err
		}
	}
	return nil
}

// recordHole records ref as a hole with holders holders, unless a put stored
// it or an audit recorded it, or nothing names it.
func recordHole(ctx context.Context, rec *Records, ref *block.BlockRef, holders uint64, res *AuditResult) error {
	// Skip a stored block, a recorded hole and one nothing names.
	listed, err := rec.HasChildren(ctx, ref)
	if err != nil || listed || holders == 0 {
		return err
	}
	hole, err := rec.IsHole(ctx, ref)
	if err != nil || hole {
		return err
	}

	// Record it, with its count at least the holders beyond the one its
	// later put implies.
	count, err := rec.GetCount(ctx, ref)
	if err != nil {
		return err
	}
	if err := rec.SetHole(ctx, ref); err != nil {
		return err
	}
	res.Holes++
	return rec.SetCount(ctx, ref, max(count, holders-1))
}
