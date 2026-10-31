package kvtx

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
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
func (v *Volume) checkDueDeadlists(ctx context.Context, mark *block_gc.BlockMark) error {
	// Gather the due records in name order, which is sequence order.
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

	// Apply each record in turn.
	for _, rec := range records {
		// Read it and apply it.
		var first *block.Deadlist
		var dead *block.Replacements
		var err error
		if v.holdCounts {
			first, dead, err = block.ReadDeadlist(ctx, v, rec.ref)
		}
		switch {
		case !v.holdCounts:
		case errors.Is(err, block.ErrNotFound):
			if le := v.logDeadlist(rec.bucketID, rec.name); le != nil {
				le.Warn("due deadlist record is missing, releasing it unapplied")
			}
		case err != nil:
			if le := v.logDeadlist(rec.bucketID, rec.name); le != nil {
				le.WithError(err).Warn("unable to read due deadlist record")
			}
			continue
		default:
			if err := v.applyDeadlist(ctx, rec.bucketID, rec.name, mark, first.GetFrom(), dead); err != nil {
				if le := v.logDeadlist(rec.bucketID, rec.name); le != nil {
					le.WithError(err).Warn("unable to apply due deadlist record")
				}
				continue
			}
		}

		// Release its name.
		err = v.withDirectAtomic(ctx, func(blocks block.StoreOps, rg *block_gc.RefGraph) error {
			return v.setBucketRoot(ctx, blocks, rg, rec.bucketID, rec.name, nil)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// applyDeadlist applies a due record in shadow: it resolves the record's
// deaths in one read, adds the shared count changes in one short write, and
// frees nothing. It counts the pages the record would free that mark reached,
// which are errors, the pages it keeps alive that mark did not reach, which
// leak, and the count changes that would go below zero, which are errors. It
// logs a sample of the errors.
//
// The read runs outside the writer, so a put may change a count before the
// write; a stale decision only skews the counters, since nothing frees.
func (v *Volume) applyDeadlist(ctx context.Context, bucketID, name string, mark *block_gc.BlockMark, from *block.BlockRef, dead *block.Replacements) error {
	// Resolve the deaths, noting the pages the mark disagrees with.
	var live []string
	var reached, leaked uint64
	var changes map[string]int64
	err := (&transactionRefGraph{volume: v}).read(ctx, func(rg *block_gc.RefGraph) error {
		var err error
		changes, err = rg.ResolveDeaths(ctx, from, dead, func(node string, frees bool) error {
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
		return err
	})
	if err != nil {
		return err
	}

	// Add the count changes.
	var underflows int
	if len(changes) != 0 {
		err := (&transactionRefGraph{volume: v}).write(ctx, func(rg *block_gc.RefGraph) error {
			var err error
			underflows, err = rg.ChangeHolds(ctx, changes)
			return err
		})
		if err != nil {
			return err
		}
	}

	// Count the record and its disagreements, logging the errors.
	v.deadlistsChecked.Add(1)
	v.deadlistLivePages.Add(reached)
	v.deadlistLeakedPages.Add(leaked)
	v.deadlistCountUnderflows.Add(uint64(underflows)) // #nosec G115 -- a count is not negative.
	if reached == 0 && underflows == 0 {
		return nil
	}
	if le := v.logDeadlist(bucketID, name); le != nil {
		le.WithField("live-pages", reached).
			WithField("sample", live).
			WithField("count-underflows", underflows).
			Error("due deadlist record would free pages the mark reached or under-counts")
	}
	return nil
}

// logDeadlist returns the log entry for a due record, or nil when the volume
// has no logger.
func (v *Volume) logDeadlist(bucketID, name string) *logrus.Entry {
	if v.le == nil {
		return nil
	}
	return v.le.WithField("bucket-id", bucketID).WithField("deadlist", name)
}
