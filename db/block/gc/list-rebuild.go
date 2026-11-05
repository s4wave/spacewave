package block_gc

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// listRebuildBatch bounds the child lists one rebuild transaction writes.
const listRebuildBatch = 4096

// ErrListRebuildAdopted is returned by a list rebuild on a Volume that is not
// unadopted: a list written outside a put records no holder in the journal,
// so only an adoption that follows may count it.
var ErrListRebuildAdopted = errors.New("the volume is adopting or adopted; rebuild child lists only before adoption")

// ListRebuild rebuilds the child lists of an unadopted Volume whose lists are
// unverified, as a store format before 7 left them. A walker that decodes
// each block by its type, which only a typed walk from the roots can know,
// hands it each block's refs through Verify; the rebuild rewrites the
// block's list when it is missing or differs and marks it verified, in
// bounded transactions that yield to writers. The marks persist, so a walk
// that stops and runs again rewrites nothing it verified. Finish then walks
// from every named root through the lists and clears the Volume's mark only
// when every listed block a root reaches is verified: a block the walker
// could not decode keeps its list unverified, and Adopt keeps refusing. A
// block no root reaches needs no verified list: a list can only miss
// children, so an unreached block's list only undercounts holders it never
// releases.
type ListRebuild struct {
	// store is the Volume.
	store Store
	// pending are the verified lists not yet written.
	pending []*block.PutBatchEntry
	// res counts the results.
	res ListRebuildResult
}

// OpenListRebuild starts a list rebuild of store, refusing a Volume that is
// not unadopted with ErrListRebuildAdopted.
func OpenListRebuild(ctx context.Context, store Store) (*ListRebuild, error) {
	err := store.View(ctx, func(rec *Records) error {
		state, err := rec.GetState(ctx)
		if err == nil && state.GetAdoption() != Adoption_ADOPTION_UNADOPTED {
			err = ErrListRebuildAdopted
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	return &ListRebuild{store: store}, nil
}

// Verify records that ref's decoded refs are refs, writing its list and mark
// with the next batch.
func (r *ListRebuild) Verify(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef) error {
	r.pending = append(r.pending, &block.PutBatchEntry{Ref: ref, Refs: refs})
	if len(r.pending) < listRebuildBatch {
		return nil
	}
	return r.flush(ctx)
}

// flush writes the pending lists and marks in one transaction while the
// Volume is unadopted, then lets a waiting writer in.
func (r *ListRebuild) flush(ctx context.Context) error {
	// Write each list that is missing or differs, and its mark.
	if len(r.pending) == 0 {
		return nil
	}
	var verified, rewritten uint64
	err := r.store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Refuse a Volume an adoption began on.
		verified, rewritten = 0, 0
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if state.GetAdoption() != Adoption_ADOPTION_UNADOPTED {
			return ErrListRebuildAdopted
		}

		// Verify each block not verified yet.
		for _, entry := range r.pending {
			done, wrote, err := r.verifyOne(ctx, rec, entry)
			if err != nil {
				return err
			}
			if done {
				verified++
			}
			if wrote {
				rewritten++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	r.res.Verified += verified
	r.res.Rewritten += rewritten
	r.pending = r.pending[:0]
	return yieldWriters(ctx)
}

// verifyOne writes entry's list when it is missing or differs from its refs
// and marks the block verified, reporting whether it verified the block and
// whether it wrote the list. A block an earlier batch or run verified is
// skipped.
func (r *ListRebuild) verifyOne(ctx context.Context, rec *Records, entry *block.PutBatchEntry) (bool, bool, error) {
	// Skip a block already verified.
	key := rec.key(kindVerified, BlockKey(entry.Ref))
	done, err := rec.tx.Exists(ctx, key)
	if err != nil || done {
		return false, false, err
	}

	// Rewrite a list that is missing or differs from the decoded refs.
	value, listed, err := rec.tx.Get(ctx, rec.key(kindChildren, BlockKey(entry.Ref)))
	if err != nil {
		return false, false, err
	}
	wrote := !listed || !bytes.Equal(value, EncodeChildren(entry.Refs))
	if wrote {
		if err := rec.PutChildren(ctx, entry.Ref, entry.Refs); err != nil {
			return false, false, err
		}
	}
	return true, wrote, rec.tx.Set(ctx, key, nil)
}

// Finish writes the pending lists, walks from every named root through the
// lists, and clears the Volume's unverified lists mark and the rebuild's
// marks when every listed block a root reaches is verified. The walk marks
// what it reaches on disk and keeps only its frontier in memory, yielding to
// writers between slices, so it runs on a live Volume; an interrupted walk
// starts over on the next Finish. A list Land writes while the mark stands is
// verified as it is written, so the World's commits during the walk add no
// unverified list; a cache fill's list is not, and a rerun of the typed walk
// verifies it.
func (r *ListRebuild) Finish(ctx context.Context) (ListRebuildResult, error) {
	// Write what is pending, then find the unverified lists a root reaches.
	if err := r.flush(ctx); err != nil {
		return r.res, err
	}
	r.res.Unverified, r.res.UnverifiedSample = 0, nil
	err := walkRoots(ctx, r.store, r.visitReached)
	if err != nil || r.res.Unverified != 0 {
		return r.res, err
	}

	// Clear the mark while the Volume is unadopted, then the rebuild's marks.
	err = r.store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Clear it only while the Volume is unadopted.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if state.GetAdoption() != Adoption_ADOPTION_UNADOPTED {
			return ErrListRebuildAdopted
		}
		state.UnverifiedLists = false
		return rec.SetState(ctx, state)
	})
	if err != nil {
		return r.res, err
	}
	r.res.Cleared = true
	return r.res, clearKinds(ctx, r.store, nil, kindVerified)
}

// visitReached counts a listed block a root reaches without a verified list,
// descending no further below it, and descends into a verified one.
func (r *ListRebuild) visitReached(ctx context.Context, rec *Records, ref *block.BlockRef) ([]*block.BlockRef, error) {
	// Step over a block without a list.
	children, listed, err := rec.GetChildren(ctx, ref)
	if err != nil || !listed {
		return nil, err
	}

	// Count an unverified list, and follow a verified one.
	verified, err := rec.tx.Exists(ctx, rec.key(kindVerified, BlockKey(ref)))
	if err != nil || verified {
		return children, err
	}
	r.res.Unverified++
	if len(r.res.UnverifiedSample) < unknownSampleSize {
		r.res.UnverifiedSample = append(r.res.UnverifiedSample, ref)
	}
	return nil, nil
}
