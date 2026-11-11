package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// walkVisit decides one block a walk reached from root, in the walk's
// transaction, and returns the children to descend into. A block two roots
// reach is visited once, from the first.
type walkVisit func(ctx context.Context, rec *Records, root *walkRoot, ref *block.BlockRef) ([]*block.BlockRef, error)

// walkRoot names the root a walk reached a block from.
type walkRoot struct {
	// bucketID is the root's bucket.
	bucketID string
	// name is the root's name in its bucket.
	name string
}

// walkEntry is a block on a walk's frontier with the root it came from.
type walkEntry struct {
	// ref is the block.
	ref *block.BlockRef
	// root is the root the walk reached it from.
	root *walkRoot
}

// walkRoots walks from every named root through visit,
// depth first, marking each block it reaches on disk so it keeps only its
// frontier in memory. It takes the roots sliceRecords at a time and drains
// the frontier below them in transactions of sliceRecords blocks, yielding to
// writers between transactions. The roots and lists may change between
// transactions, so it passes over the roots again until a pass reaches no
// block it had not, which also reaches a block a root moved onto during the
// walk. The named roots are every holder record: the leased roots, reader
// pins and stages are named roots too. It keeps the marks it writes, adding to
// the marks it finds, for the caller to read and delete; a caller starting
// over deletes them first, since an interrupted walk leaves marks whose
// children it never reached. It returns the state's old holds as the last
// pass read them before it began. Each pass starts by advancing the state's
// list pass, as LivenessState.list_pass describes.
func walkRoots(ctx context.Context, store Store, visit walkVisit) (uint64, error) {
	for {
		holds, reached, err := walkPass(ctx, store, visit)
		if err != nil || reached == 0 {
			return holds, err
		}
	}
}

// walkPass walks from each named root once, returning the state's old holds
// before it began and how many blocks it reached that no earlier pass had.
func walkPass(ctx context.Context, store Store, visit walkVisit) (uint64, uint64, error) {
	// Advance the list pass and read the old holds before the pass, then
	// walk from each root.
	var holds, reached uint64
	err := store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Read the state, then take its holds and advance its pass.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		holds = state.GetOldHolds()
		state.ListPass++
		return rec.SetState(ctx, state)
	})
	if err != nil {
		return 0, 0, err
	}
	var after []byte
	for {
		// Take the next roots.
		var frontier []walkEntry
		var n int
		err := store.View(ctx, func(rec *Records) error {
			// Read the slice, taking each root's block.
			recs, err := rec.scanAfter(ctx, kindRoot, after, sliceRecords)
			if err != nil {
				return err
			}
			n = len(recs)
			for _, r := range recs {
				bucketID, name, err := parseNamedKey(r.key)
				if err != nil {
					return err
				}
				entry := &RootEntry{}
				if err := entry.UnmarshalVT(r.value); err != nil {
					return err
				}
				ref, _, err := ParseBlockKey(entry.GetRef())
				if err != nil {
					return err
				}
				frontier = append(frontier, walkEntry{ref: ref, root: &walkRoot{bucketID: bucketID, name: name}})
				after = r.key
			}
			return nil
		})
		if err != nil {
			return holds, reached, err
		}

		// Drain the frontier below them, sliceRecords blocks per
		// transaction off its top: mark and visit each the walk has not
		// reached, and queue its children.
		_, err = eachSlice(ctx, store.Update, frontier, nil, func(rec *Records, _ block.StoreOps, next []walkEntry) ([]walkEntry, bool, error) {
			for taken := 0; len(next) != 0 && taken < sliceRecords; taken++ {
				// Take the next block, skipping one the walk reached.
				e := next[len(next)-1]
				next = next[:len(next)-1]
				key := rec.key(kindReached, BlockKey(e.ref))
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
				children, err := visit(ctx, rec, e.root, e.ref)
				if err != nil {
					return nil, false, err
				}
				for _, child := range children {
					next = append(next, walkEntry{ref: child, root: e.root})
				}
			}
			return next, len(next) != 0, nil
		})
		if err != nil {
			return holds, reached, err
		}
		if n < sliceRecords {
			break
		}
	}
	return holds, reached, nil
}
