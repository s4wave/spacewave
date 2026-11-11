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

// ErrListsVerified is returned by a list rebuild on a Volume whose old
// generation is no longer Open: the settle flipped it, so every list a holder
// reaches is marked, and a rewrite would change a list the sweep reads.
var ErrListsVerified = errors.New("the volume's old generation has settled; there is nothing to rebuild")

// rebuildable returns the error a list rebuild gets on a Volume in state, or
// nil when it may rewrite lists.
func rebuildable(state *LivenessState) error {
	switch {
	case state.GetAdoption() == Adoption_ADOPTION_ADOPTING:
		return ErrListRebuildAdopting
	case state.GetGeneration() != Generation_GENERATION_OPEN:
		return ErrListsVerified
	}
	return nil
}

// ListRebuild is the typed walk of the settle of a Volume's old generation,
// as settle.go describes: a walker that decodes each block by its type,
// which only a walk from the roots can know, hands it each block's refs
// through Verify, and the rebuild writes the block's list from them, marked,
// in bounded transactions that yield to writers. Writing a marked list takes
// the block off the run and the front and pushes its unverified children
// onto the front, so the front holds what the walk still owes and the
// settle flips only once it is empty. A rerun skips every marked list.
//
// Verify writes the union of the stored list and the refs, since a list may
// lack a child and naming one more only keeps a block longer; refs that lack
// a child the stored list names are refused, not written: a stored list can
// only miss children, so such refs come from a wrong decoder, and the block
// stays on the front. A list that gains a child journals no holder for it,
// so on a Volume that keeps a journal the rewrite records that the counts
// owe a recount, which the flip reads to return the Volume to unadopted
// first.
//
// It runs on an unadopted or an adopted Volume, but not while an adoption
// counts, and only while the old generation is Open.
type ListRebuild struct {
	// store is the Volume.
	store Store
	// pending are the decoded refs not yet written.
	pending []*block.PutBatchEntry
	// res counts the results.
	res ListRebuildResult
}

// OpenListRebuild starts a list rebuild of store, refusing a Volume an
// adoption is counting with ErrListRebuildAdopting and one whose old
// generation is not Open with ErrListsVerified.
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

// Verify records that ref's decoded refs are refs, writing its list with
// the next batch.
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
	// listSkipped is a block whose list is marked already.
	listSkipped listVerdict = iota
	// listVerified is a block whose stored list named every child its refs
	// name.
	listVerified
	// listRewritten is a block whose list was missing or lacked a child.
	listRewritten
	// listDropped is a block whose refs lack a child its stored list names.
	listDropped
)

// flush writes the pending lists in one transaction while no adoption
// counts and the generation is Open, then lets a waiting writer in.
func (r *ListRebuild) flush(ctx context.Context) error {
	// Nothing is pending.
	if len(r.pending) == 0 {
		return nil
	}

	// Write each list from its refs.
	var res ListRebuildResult
	err := r.store.Update(ctx, func(rec *Records, _ block.StoreOps) error {
		// Refuse a Volume an adoption is counting or the settle flipped.
		res = ListRebuildResult{DroppedSample: r.res.DroppedSample}
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if err := rebuildable(state); err != nil {
			return err
		}

		// Verify each block, then record a recount the counts owe.
		var rewrote bool
		for _, entry := range r.pending {
			verdict, err := verifyOne(ctx, rec, entry)
			if err != nil {
				return err
			}
			switch verdict {
			case listRewritten:
				res.Rewritten++
				rewrote = true
				fallthrough
			case listVerified:
				res.Verified++
			case listDropped:
				res.Dropped++
				res.DroppedSample = sample(res.DroppedSample, entry.Ref)
			}
		}
		if !rewrote {
			return nil
		}
		return rec.noteRewrite(ctx, state)
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

// verifyOne writes the list of the block entry names from its decoded refs,
// marked, while the generation is Open: a missing list from the refs, an
// unverified one as the union, as mergeList describes. A marked list is
// skipped, and a refused one is left, on the front.
func verifyOne(ctx context.Context, rec *Records, entry *block.PutBatchEntry) (listVerdict, error) {
	// Skip a marked list, which leaves the front when it is marked.
	key := BlockKey(entry.Ref)
	value, listed, err := rec.tx.Get(ctx, rec.key(kindChildren, key))
	if err != nil || listed && listMarked(value) {
		return listSkipped, err
	}

	// Write a missing list from the refs, or merge an unverified one.
	var packed []byte
	verdict := listRewritten
	if !listed {
		packed = EncodeChildren(entry.Refs)
		if err := rec.tx.Set(ctx, rec.key(kindChildren, key), markList(packed)); err != nil {
			return listSkipped, err
		}
	} else {
		var gained, dropped bool
		packed, gained, dropped, err = rec.mergeList(ctx, key, value, entry.Refs, true)
		if err != nil || dropped {
			return listDropped, err
		}
		if !gained {
			verdict = listVerified
		}
	}

	// Keep the front invariant.
	return verdict, rec.markedYoung(ctx, key, packed, nil)
}

// Finish writes the pending lists and reports the blocks the front still
// holds: the unverified blocks a holder reaches that no Verify wrote, such
// as a block the walker could not decode or one whose refs were refused. It
// counts them in slices that each read sliceRecords front entries, so it
// yields to writers on a live Volume. The settle flips once the front is
// empty and the run is complete.
func (r *ListRebuild) Finish(ctx context.Context) (ListRebuildResult, error) {
	// Write what is pending.
	if err := r.flush(ctx); err != nil {
		return r.res, err
	}

	// Count the front in slices, adding each slice once its view ends.
	r.res.Front, r.res.FrontSample = 0, nil
	var from []byte
	for {
		var next []byte
		var keys [][]byte
		err := r.store.View(ctx, func(rec *Records) error {
			next, keys = nil, keys[:0]
			return rec.scan(ctx, rec.key(kindFront), from, func(key []byte, _ func() ([]byte, error)) (bool, error) {
				if len(keys) == sliceRecords {
					next = bytes.Clone(key)
					return false, nil
				}
				keys = append(keys, bytes.Clone(key))
				return true, nil
			})
		})
		if err != nil {
			return r.res, err
		}
		for _, key := range keys {
			r.res.Front++
			if len(r.res.FrontSample) < sampleSize {
				ref, _, err := ParseBlockKey(key)
				if err != nil {
					return r.res, err
				}
				r.res.FrontSample = append(r.res.FrontSample, ref)
			}
		}
		if next == nil {
			return r.res, nil
		}
		from = next
	}
}

// unionChildren returns the encoded child list naming every child of the
// encoded lists a and b, which hold distinct block keys in bytes.Compare
// order, as EncodeChildren writes them.
func unionChildren(a, b []byte) ([]byte, error) {
	out := make([]byte, 0, max(len(a), len(b)))
	for len(a) != 0 || len(b) != 0 {
		x, okA := nextBlockKey(a)
		y, okB := nextBlockKey(b)
		switch {
		case len(a) != 0 && !okA, len(b) != 0 && !okB:
			return nil, errBadBlockKey
		case !okB || okA && bytes.Compare(x, y) < 0:
			out, a = append(out, x...), a[len(x):]
		case !okA || bytes.Compare(x, y) > 0:
			out, b = append(out, y...), b[len(y):]
		default:
			out, a, b = append(out, x...), a[len(x):], b[len(y):]
		}
	}
	return out, nil
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
