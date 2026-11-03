package kvtx

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/sirupsen/logrus"
)

// rootOwnerKind is the owner kind of the named bucket roots, which a block
// pass keeps from its mark to find the due deadlist records.
var rootOwnerKind = strings.TrimSuffix(RootOwnerPrefix, ":")

// deadlistSampleSize bounds the live pages logged for one record.
const deadlistSampleSize = 8

// checkDueDeadlists applies each due deadlist record a bucket names under
// block.DeadlistDueRootPrefix in shadow, in name order, against mark, which
// read every child list and kept the named roots' edges. Applying a record
// writes the shared count changes it makes and deletes nothing; a page it
// would free that the mark reached is a deadlist error, counted and logged.
// Each applied record's name is released, so a later pass sweeps it. A record
// the store no longer holds, or that a Volume keeping no shared counts cannot
// check, is released unapplied; one that fails to read or apply, such as
// while the sharing walk builds the counts, stays for the next pass.
func (v *Volume) checkDueDeadlists(ctx context.Context, mark *block_gc.BlockMark) (bool, error) {
	// Keep the bounded roots the mark already read, in sequence order.
	type due struct {
		bucketID, name string
		ref            *block.BlockRef
	}
	var records []due
	for _, edge := range mark.OwnerEdges {
		bucketID, name, ok := parseRootOwnerIRI(edge.Subject)
		if !ok || !strings.HasPrefix(name, block.DeadlistDueRootPrefix) {
			continue
		}
		if ref, ok := block_gc.ParseBlockIRI(edge.Object); ok {
			records = append(records, due{bucketID: bucketID, name: name, ref: ref})
		}
	}
	slices.SortFunc(records, func(a, b due) int {
		return cmp.Or(strings.Compare(a.bucketID, b.bucketID), strings.Compare(a.name, b.name))
	})

	// Unknown history needs only its immutable header, never a whole record
	// read or a complete mark. Its own missing metadata must not hold it due.
	changed := false
	for _, rec := range records {
		first, err := block.ReadDeadlistBlock(ctx, v, rec.ref)
		if !v.holdCounts || errors.Is(err, block.ErrNotFound) {
			retired := false
			err = v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
				// Retire only the exact name this mark observed.
				current, err := rg.GetOutgoingRefs(ctx, RootOwnerIRI(rec.bucketID, rec.name))
				if err != nil || !slices.Contains(current, block_gc.BlockIRI(rec.ref)) {
					return err
				}
				if err := v.setBucketRoot(ctx, blocks, rg, rec.bucketID, rec.name, nil); err != nil {
					return err
				}
				retired = true
				return nil
			})
			if err != nil {
				return changed, err
			}
			changed = changed || retired
			continue
		}
		if err != nil {
			if le := v.logDeadlist(rec.bucketID, rec.name); le != nil {
				le.WithError(err).Warn("unable to read due deadlist header")
			}
			continue
		}

		// Aggregate only an applicable small record, outside the writer.
		dead := &block.Replacements{CountsComplete: first.CountsComplete, CountingGeneration: first.CountingGeneration}
		trusted := false
		err = (&transactionRefGraph{volume: v}).read(ctx, func(rg *block_gc.RefGraph) error {
			var err error
			trusted, err = deadlistBasisCurrent(ctx, rg, dead)
			return err
		})
		large := false
		if err == nil && trusted {
			var small bool
			_, parsed, small, readErr := block.ReadSmallDeadlist(ctx, v, rec.ref)
			// Cache-byte eviction may preserve the basis but lose a historical
			// list body. Abandon that unreadable history through the same
			// bounded recovery as an oversized record; never strand its name.
			switch {
			case errors.Is(readErr, block.ErrNotFound):
				large = true
			case readErr != nil:
				err = readErr
			case small:
				dead = parsed
			default:
				large = true
			}
		}
		if err == nil {
			var retired bool
			retired, err = v.applyDeadlistPlan(ctx, rec.bucketID, rec.name, rec.ref, mark, first.From, dead, large)
			changed = changed || retired
		}
		if err != nil && v.le != nil {
			v.logDeadlist(rec.bucketID, rec.name).WithError(err).Warn("unable to apply due deadlist record")
		}
	}
	return changed, nil
}

// applyDeadlist applies a small explicit plan, retrying a changed revision once
// off the writer. Continued foreground writes retain the due name unchanged.
func (v *Volume) applyDeadlist(ctx context.Context, bucketID, name string, ref *block.BlockRef, mark *block_gc.BlockMark, from *block.BlockRef, dead *block.Replacements) error {
	_, err := v.applyDeadlistPlan(ctx, bucketID, name, ref, mark, from, dead, dead.RefCount() > block.MaxDeadlistPartRefs)
	return err
}

// applyDeadlistPlan reports only committed retirement, including safe history
// abandonment. Stale attempts publish no deltas, queues, names or diagnostics.
func (v *Volume) applyDeadlistPlan(ctx context.Context, bucketID, name string, ref *block.BlockRef, mark *block_gc.BlockMark, from *block.BlockRef, dead *block.Replacements, large bool) (bool, error) {
	for range 2 {
		retired, err := v.applyDeadlistSnapshot(ctx, bucketID, name, ref, mark, from, dead, large)
		if !errors.Is(err, errPhysicalRevisionChanged) {
			return retired, err
		}
	}
	return false, nil
}

// applyDeadlistSnapshot never reconstructs erased historical holders. Only a
// small record with its unchanged complete basis may publish precise deltas or
// settle the lineage through its fates and return provisional Found claims.
// Rebuild/uncertainty abandons all history;
// ordinary record children retain Found until the record itself is collected.
func (v *Volume) applyDeadlistSnapshot(ctx context.Context, bucketID, name string, ref *block.BlockRef, mark *block_gc.BlockMark, from *block.BlockRef, dead *block.Replacements, large bool) (bool, error) {
	// Resolve applicable bounded history against one revision, off the writer.
	var live []string
	var reached, leaked uint64
	var changes map[string]int64
	var revision uint64
	resolved, recovery := false, large
	err := (&transactionRefGraph{volume: v}).readSnapshot(ctx, func(tx kvtx.Tx, rg *block_gc.RefGraph) error {
		// Capture the accounting and physical revision from the same snapshot.
		var err error
		if revision, err = readPhysicalRevision(ctx, tx, v.physicalRevisionKey()); err != nil {
			return err
		}
		trusted, err := deadlistBasisCurrent(ctx, rg, dead)
		if err != nil || !trusted {
			return err
		}
		if large {
			resolved = true
			return nil
		}
		if mark.Unknown != 0 || !mark.Awaitable() {
			return nil
		}

		// Fate decisions retain the transfer holders through child funding.
		var parent bool
		changes, parent, err = rg.ResolveDeathsBounded(ctx, from, dead, block.MaxDeadlistPartRefs, func(node string, frees bool) error {
			switch isReached := mark.Live.Has(node); {
			case frees && isReached:
				if reached++; len(live) < deadlistSampleSize {
					live = append(live, node)
				}
			case !frees && !isReached:
				leaked++
			}
			return nil
		})
		if errors.Is(err, block_gc.ErrDeathWalkBudget) {
			changes, live, reached, leaked = nil, nil, 0, 0
			resolved, recovery = true, true
			return nil
		}
		resolved = err == nil
		recovery = parent || len(changes) > block.MaxDeadlistPartRefs
		return err
	})
	if err != nil {
		return false, err
	}

	// Validate the snapshot and retire the exact due root atomically. Large
	// deltas or a retained parent start recount, never a long scalar loop.
	var underflows int
	retired := false
	err = v.withDirectAtomicAt(ctx, &revision, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
		// A rename or changed basis invalidates every old fate decision.
		current, err := rg.GetOutgoingRefs(ctx, RootOwnerIRI(bucketID, name))
		if err != nil || !slices.Contains(current, block_gc.BlockIRI(ref)) {
			return err
		}
		trusted, err := deadlistBasisCurrent(ctx, rg, dead)
		if err != nil || (trusted && !resolved) {
			return err
		}

		// Only the precise path returns Found after funding child edges.
		// ResolveDeaths already settles From: its listed fate or Existed
		// decrement returns it, or a fresh parent's edge inherits the lineage.
		if trusted && !recovery {
			if underflows, err = rg.ChangeHolds(ctx, changes); err != nil {
				return err
			}
			returns := make([]string, 0, len(dead.Found))
			for _, page := range dead.Found {
				returns = append(returns, block_gc.BlockIRI(page))
			}
			if err := rg.ReleaseHolds(ctx, returns); err != nil {
				return err
			}
		} else {
			reached, leaked, live = 0, 0, nil
		}

		// First publish the final graph. Recount replaces scalar history from
		// that graph and all surviving physical children before trust resumes.
		if err := v.setBucketRoot(ctx, blocks, rg, bucketID, name, nil); err != nil {
			return err
		}
		if trusted && recovery {
			if _, err := rg.RecountSharing(ctx); err != nil {
				return err
			}
		}
		retired = true
		return nil
	})
	if err != nil || !retired {
		return false, err
	}

	// Diagnostics describe only the committed precise plan, not abandoned
	// or stale history whose scalar changes never executed.
	v.deadlistsChecked.Add(1)
	v.deadlistLivePages.Add(reached)
	v.deadlistLeakedPages.Add(leaked)
	v.deadlistCountUnderflows.Add(uint64(underflows)) // #nosec G115 -- a count is not negative.
	if (reached != 0 || underflows != 0) && v.le != nil {
		v.logDeadlist(bucketID, name).WithField("live-pages", reached).
			WithField("sample", live).WithField("count-underflows", underflows).
			Error("due deadlist record would free reached pages or under-count")
	}
	return true, nil
}

// logDeadlist returns the log entry for a due record, or nil when the volume
// has no logger.
func (v *Volume) logDeadlist(bucketID, name string) *logrus.Entry {
	if v.le == nil {
		return nil
	}
	return v.le.WithField("bucket-id", bucketID).WithField("deadlist", name)
}
