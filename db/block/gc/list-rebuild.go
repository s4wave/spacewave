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
	// pending are the verified lists not yet written.
	pending []*block.PutBatchEntry
	// res counts the results.
	res ListRebuildResult
}

// OpenListRebuild starts a list rebuild of store, refusing a Volume an
// adoption is counting with ErrListRebuildAdopting and one whose lists are
// verified with ErrListsVerified. A Finish that returns the Volume to
// unadopted reports it through Records.Unadopted, so store starts the
// recount.
func OpenListRebuild(ctx context.Context, store Store) (*ListRebuild, error) {
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

// verifyOne checks entry's refs against the block's stored list, as
// checkList does, and marks the block verified unless the refs drop a child.
func (r *ListRebuild) verifyOne(ctx context.Context, rec *Records, entry *block.PutBatchEntry) (listVerdict, error) {
	// Skip a block already verified, or one Land wrote while the mark stood.
	key := BlockKey(entry.Ref)
	done, err := rec.listVerified(ctx, key)
	if err != nil || done {
		return listSkipped, err
	}

	// Check the list, and mark it.
	verdict, err := rec.checkList(ctx, key, entry.Refs)
	if err != nil || verdict == listDropped {
		return verdict, err
	}
	return verdict, rec.tx.Set(ctx, rec.key(kindVerified, key), nil)
}

// checkList checks refs, the decoded refs of the block at key, against its
// stored child list: it refuses refs that lack a child the list names, since
// a list can only miss children, writes the list when it is missing or lacks
// a child the refs name, and leaves a matching one. A rewrite journals no
// holder for a child the list gains; the caller owes the counts a recount.
func (r *Records) checkList(ctx context.Context, key []byte, refs []*block.BlockRef) (listVerdict, error) {
	// Read the stored list.
	value, listed, err := r.tx.Get(ctx, r.key(kindChildren, key))
	if err != nil {
		return listSkipped, err
	}

	// Keep a matching list, refuse refs that drop a child, and rewrite the
	// rest.
	encoded := EncodeChildren(refs)
	switch {
	case listed && bytes.Equal(value, encoded):
		return listVerified, nil
	case listed && !containsChildren(encoded, value):
		return listDropped, nil
	}
	return listRewritten, r.tx.Set(ctx, r.key(kindChildren, key), encoded)
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
//
// A listed block no root reaches keeps its unverified list past the clear,
// and a later put of it revives it with that list. Before the clear, Finish
// therefore writes a suspect record for each listed block no walk verified
// and no Land wrote, and Land checks a suspect list against the put's refs
// before it trusts it, as Land describes.
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

	// Name the lists nothing verified.
	if err := r.markSuspects(ctx); err != nil {
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

		// A block reaches zero only by netting a record sequenced after
		// every unnetted up pair it has; a parked block's zero came from
		// lists that may have been short, so it is freed only after every
		// record journaled while they stood has netted.
		state.HoldMark = state.GetNextSeq()

		// Return an adopted Volume a rewrite left short to unadopted.
		recount, err := rec.tx.Exists(ctx, rec.key(kindRewritten))
		if err != nil {
			return err
		}
		if recount && state.GetAdoption() == Adoption_ADOPTION_ADOPTED {
			rec.unadopt(state)
			r.res.Unadopted = true
		}
		return rec.SetState(ctx, state)
	})
	if err != nil {
		return r.res, err
	}
	r.res.Cleared = true

	// Delete the rebuild's marks.
	return r.res, clearKinds(ctx, r.store, nil, kindVerified, kindLanded)
}

// markSuspects writes a suspect record for each listed block without a
// verified or landed mark, an anti-join of the child lists against the
// marks, in slices of sliceRecords lists that let a waiting writer in. A put
// between slices needs no fence: one of a block the join has passed checks
// the list and deletes the record, and one of a block ahead of it writes a
// mark the join then finds.
func (r *ListRebuild) markSuspects(ctx context.Context) error {
	r.res.Suspect = 0
	_, err := eachSlice(ctx, r.store.Update, []byte(nil), nil, func(rec *Records, _ block.StoreOps, after []byte) ([]byte, bool, error) {
		// Read the next slice of listed blocks, keys only.
		from := after
		if after != nil {
			from = append(bytes.Clone(after), 0)
		}
		var keys [][]byte
		err := rec.scan(ctx, rec.key(kindChildren), from, func(key []byte, _ func() ([]byte, error)) (bool, error) {
			if len(keys) == sliceRecords {
				return false, nil
			}
			keys = append(keys, bytes.Clone(key))
			return true, nil
		})
		if err != nil || len(keys) == 0 {
			return nil, false, err
		}

		// Name each one nothing verified.
		for _, key := range keys {
			verified, err := rec.listVerified(ctx, key)
			if err != nil {
				return nil, false, err
			}
			if verified {
				continue
			}
			r.res.Suspect++
			if err := rec.tx.Set(ctx, rec.key(kindSuspect, key), nil); err != nil {
				return nil, false, err
			}
		}
		return keys[len(keys)-1], len(keys) == sliceRecords, nil
	})
	return err
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
