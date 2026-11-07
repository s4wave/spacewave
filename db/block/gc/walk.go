package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// walkVisit decides one block a walk reached, in the walk's transaction, and
// returns the children to descend into.
type walkVisit func(ctx context.Context, rec *Records, ref *block.BlockRef) ([]*block.BlockRef, error)

// walkRoots walks from every named root through visit, depth first, marking
// each block it reaches on disk so it keeps only its frontier in memory. It
// takes the roots sliceRecords at a time and drains the frontier below them
// in transactions of sliceRecords blocks, yielding to writers between
// transactions. The roots and lists may change between transactions, so it
// passes over the roots again until a pass reaches no block it had not, which
// also reaches a block a root moved onto during the walk. It deletes the marks
// when it starts, so a walk an interruption stopped starts over, and when it
// ends.
func walkRoots(ctx context.Context, store Store, visit walkVisit) error {
	// Start from no marks.
	if err := clearKinds(ctx, store, nil, kindReached); err != nil {
		return err
	}

	// Pass over the roots until a pass reaches nothing new.
	for {
		reached, err := walkPass(ctx, store, visit)
		if err != nil {
			return err
		}
		if reached == 0 {
			break
		}
	}
	return clearKinds(ctx, store, nil, kindReached)
}

// walkPass walks from each named root once, returning how many blocks it
// reached that no earlier pass had.
func walkPass(ctx context.Context, store Store, visit walkVisit) (uint64, error) {
	var reached uint64
	var after []byte
	for {
		// Take the next roots.
		var frontier []*block.BlockRef
		var n int
		err := store.View(ctx, func(rec *Records) error {
			// Read the next root records after the last one taken.
			recs, err := rec.scanAfter(ctx, kindRoot, after, sliceRecords)
			if err != nil {
				return err
			}
			n = len(recs)
			for _, r := range recs {
				entry := &RootEntry{}
				if err := entry.UnmarshalVT(r.value); err != nil {
					return err
				}
				ref, _, err := ParseBlockKey(entry.GetRef())
				if err != nil {
					return err
				}
				frontier = append(frontier, ref)
				after = r.key
			}
			return nil
		})
		if err != nil {
			return reached, err
		}

		// Drain the frontier below them, sliceRecords blocks per transaction
		// off its top: mark and visit each the walk has not reached, and
		// queue its children.
		_, err = eachSlice(ctx, store.Update, frontier, nil, func(rec *Records, _ block.StoreOps, next []*block.BlockRef) ([]*block.BlockRef, bool, error) {
			for taken := 0; len(next) != 0 && taken < sliceRecords; taken++ {
				// Take the next block, skipping one the walk reached.
				ref := next[len(next)-1]
				next = next[:len(next)-1]
				key := rec.key(kindReached, BlockKey(ref))
				seen, err := rec.tx.Exists(ctx, key)
				if err != nil {
					return nil, false, err
				}
				if seen {
					continue
				}

				// Mark it, visit it, and queue its children.
				if err := rec.tx.Set(ctx, key, nil); err != nil {
					return nil, false, err
				}
				reached++
				children, err := visit(ctx, rec, ref)
				if err != nil {
					return nil, false, err
				}
				next = append(next, children...)
			}
			return next, len(next) != 0, nil
		})
		if err != nil || n < sliceRecords {
			return reached, nil
		}
	}
}
