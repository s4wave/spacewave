package block_gc

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// listRebuildBatch bounds the child lists one rebuild transaction writes.
const listRebuildBatch = 4096

// ErrListRebuildAdopting is returned by a list rebuild on a Volume an
// adoption is counting: the count may already have read the list.
var ErrListRebuildAdopting = errors.New("the volume is adopting; rebuild child lists before or after an adoption")

// ErrListsVerified is returned by a list rebuild on a Volume whose
// unverified lists mark is clear: its lists are verified, and a rewrite would
// leave the counts short with no Finish to recount them.
var ErrListsVerified = errors.New("the volume's child lists are verified; there is nothing to rebuild")

// rebuildable returns the error a list rebuild gets on a Volume in state, or
// nil when it may rewrite lists.
func rebuildable(state *LivenessState) error {
	switch {
	case state.GetAdoption() == Adoption_ADOPTION_ADOPTING:
		return ErrListRebuildAdopting
	case !state.GetUnverifiedLists():
		return ErrListsVerified
	}
	return nil
}

// ListRebuild rebuilds the child lists of a Volume whose lists are
// unverified, as a store format before 7 left them. A walker that decodes
// each block by its type, which only a typed walk from the roots can know,
// hands it each block's refs through Verify; the rebuild rewrites the
// block's list when it is missing or lacks a child the refs name and marks
// it verified, in bounded transactions that yield to writers. The marks
// persist, so a walk that stops and runs again rewrites nothing it verified.
// Finish then walks from every named root through the lists and clears the
// Volume's mark only when every listed block a root reaches is verified: a
// block the walker could not decode keeps its list unverified, and the
// collector keeps freeing only blocks Land gave birth to since the upgrade.
// A block no root reaches needs no verified list: a list can only miss
// children, so an unreached block's list only undercounts holders it never
// releases.
//
// It runs on an unadopted or an adopted Volume, but not while an adoption
// counts. It journals no holder for a child a rewritten list gains, so the
// counts of an adopted Volume lack those holders: the rewrite records that a
// recount is due, kindRewritten, in its own transaction, and Finish returns
// such a Volume to unadopted in the transaction that clears the mark, so
// nothing more frees until an adoption recounts every list. An unadopted
// Volume needs no record, since the next adoption counts the lists as they
// are. A decoded list that lacks a child the stored list names is refused,
// not written: a stored list can only miss children, so the report is wrong,
// and the block stays unverified.
type ListRebuild struct {
	// store is the Volume.
	store Store
	// unadopted is called after Finish returns the Volume to unadopted, nil
	// when nothing waits on it.
	unadopted func()
	// pending are the verified lists not yet written.
	pending []*block.PutBatchEntry
	// res counts the results.
	res ListRebuildResult
}

// OpenListRebuild starts a list rebuild of store, refusing a Volume an
// adoption is counting with ErrListRebuildAdopting and one whose lists are
// verified with ErrListsVerified. Finish calls unadopted, unless it is nil,
// once it has returned the Volume to unadopted, so the owner of the Volume's
// adoption can start the recount.
func OpenListRebuild(ctx context.Context, store Store, unadopted func()) (*ListRebuild, error) {
	err := store.View(ctx, func(rec *Records) error {
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		return rebuildable(state)
	})
	if err != nil {
		return nil, err
	}
	return &ListRebuild{store: store, unadopted: unadopted}, nil
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

// listVerdict is what verifyOne did with one block's decoded refs.
type listVerdict int

const (
	// listSkipped is a block an earlier batch or run verified, or one Land
	// wrote while the mark stood.
	listSkipped listVerdict = iota
	// listVerified is a block whose stored list matched its refs.
	listVerified
	// listRewritten is a block whose list was missing or lacked a child.
	listRewritten
	// listDropped is a block whose refs lack a child its stored list names.
	listDropped
)

// flush writes the pending lists and marks in one transaction while no
// adoption counts and the mark stands, with the record that a recount is due
// when it rewrote a list on an adopted Volume, then lets a waiting writer in.
func (r *ListRebuild) flush(ctx context.Context) error {
	// Nothing is pending.
	if len(r.pending) == 0 {
		return nil
	}

	// Write each list that is missing or short, and its mark.
	var res ListRebuildResult
	err := r.store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Refuse a Volume an adoption is counting or a Finish verified.
		res = ListRebuildResult{DroppedSample: r.res.DroppedSample}
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if err := rebuildable(state); err != nil {
			return err
		}

		// Verify each block not verified yet.
		for _, entry := range r.pending {
			verdict, err := r.verifyOne(ctx, rec, entry)
			if err != nil {
				return err
			}
			switch verdict {
			case listRewritten:
				res.Rewritten++
				fallthrough
			case listVerified:
				res.Verified++
			case listDropped:
				res.Dropped++
				res.DroppedSample = sample(res.DroppedSample, entry.Ref)
			}
		}

		// Record that the counts of an adopted Volume lack the new holders.
		if res.Rewritten == 0 || state.GetAdoption() != Adoption_ADOPTION_ADOPTED {
			return nil
		}
		return rec.tx.Set(ctx, rec.key(kindRewritten), nil)
	})
	if err != nil {
		return err
	}

	// Add the batch's tally and let a waiting writer in.
	r.res.Verified += res.Verified
	r.res.Rewritten += res.Rewritten
	r.res.Dropped += res.Dropped
	r.res.DroppedSample = res.DroppedSample
	r.pending = r.pending[:0]
	return context.Cause(ctx)
}

// verifyOne checks entry's refs against the block's stored list: it refuses
// refs that lack a child the list names, writes the list when it is missing
// or lacks a child the refs name, and marks the block verified otherwise.
func (r *ListRebuild) verifyOne(ctx context.Context, rec *Records, entry *block.PutBatchEntry) (listVerdict, error) {
	// Skip a block already verified, or one Land wrote while the mark stood.
	key := BlockKey(entry.Ref)
	done, err := rec.listVerified(ctx, key)
	if err != nil || done {
		return listSkipped, err
	}

	// Refuse refs that drop a stored child, and rewrite a list they extend.
	value, listed, err := rec.tx.Get(ctx, rec.key(kindChildren, key))
	if err != nil {
		return listSkipped, err
	}
	encoded := EncodeChildren(entry.Refs)
	verdict := listVerified
	if !listed || !bytes.Equal(value, encoded) {
		if listed && !containsChildren(encoded, value) {
			return listDropped, nil
		}
		if err := rec.tx.Set(ctx, rec.key(kindChildren, key), encoded); err != nil {
			return listSkipped, err
		}
		verdict = listRewritten
	}
	return verdict, rec.tx.Set(ctx, rec.key(kindVerified, key), nil)
}

// Finish writes the pending lists, walks from every named root through the
// lists, and clears the Volume's unverified lists mark and the rebuild's
// marks when every listed block a root reaches is verified. The walk marks
// what it reaches on disk and keeps only its frontier in memory, yielding to
// writers between slices, so it runs on a live Volume; an interrupted walk
// starts over on the next Finish. A list Land writes while the mark stands is
// verified as it is written, by its landed mark, so the World's commits during the walk add no
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

	// Clear the mark while no adoption counts, then the rebuild's marks.
	r.res.Unadopted = false
	err = r.store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Clear it only while no adoption counts.
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if state.GetAdoption() == Adoption_ADOPTION_ADOPTING {
			return ErrListRebuildAdopting
		}
		state.UnverifiedLists = false

		// Return an adopted Volume a rewrite left short to unadopted under
		// no run, as a claim does, ending any release in progress.
		recount, err := rec.tx.Exists(ctx, rec.key(kindRewritten))
		if err != nil {
			return err
		}
		if recount && state.GetAdoption() == Adoption_ADOPTION_ADOPTED {
			state.Adoption = Adoption_ADOPTION_UNADOPTED
			state.AdoptionRun = ""
			state.Release = nil
			r.res.Unadopted = true
		}
		return rec.SetState(ctx, state)
	})
	if err != nil {
		return r.res, err
	}
	r.res.Cleared = true

	// Delete the rebuild's marks, then signal the recount, which an unadopted
	// Volume needs even when the marks remain.
	err = clearKinds(ctx, r.store, nil, kindVerified, kindLanded)
	if r.res.Unadopted && r.unadopted != nil {
		r.unadopted()
	}
	return r.res, err
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
	verified, err := rec.listVerified(ctx, BlockKey(ref))
	if err != nil || verified {
		return children, err
	}
	r.res.Unverified++
	r.res.UnverifiedSample = sample(r.res.UnverifiedSample, ref)
	return nil, nil
}

// containsChildren reports whether the encoded child list has holds every
// child of the encoded list want. Both hold distinct block keys in
// bytes.Compare order, as EncodeChildren writes them.
func containsChildren(has, want []byte) bool {
	for len(want) != 0 {
		w, ok := nextBlockKey(want)
		if !ok {
			return false
		}
		for {
			h, ok := nextBlockKey(has)
			if !ok {
				return false
			}
			has = has[len(h):]
			if c := bytes.Compare(h, w); c == 0 {
				break
			} else if c > 0 {
				return false
			}
		}
		want = want[len(w):]
	}
	return true
}

// nextBlockKey returns the block key at the start of b, as ParseBlockKey
// reads it, without decoding it.
func nextBlockKey(b []byte) ([]byte, bool) {
	if len(b) == 0 {
		return nil, false
	}
	size := hash.HashType(b[0]).GetHashLen()
	if size <= 0 || len(b)-1 < size {
		return nil, false
	}
	return b[:1+size], true
}
