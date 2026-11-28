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

// ListRebuild is the typed walk of the settle of a Volume's old generation, as
// settle.go describes: a walker that decodes each block by its type, which only
// a walk from the roots can know, hands it each block's refs through Verify,
// and the rebuild writes the block's list from them, marked, in bounded
// transactions that yield to writers. Writing a marked list takes the block off
// the run and the front and pushes its unverified children onto the front, so
// the front holds what the walk still owes and the settle flips only once the
// front is empty.
//
// A marked list is compared too, since Land and the rebuild mark a list with
// whatever refs their caller hands them, and the walker can verify one block
// twice in a run, under two types: the first, short, marks the list, and the
// second must still add the children it lacks. A marked list that lacks a
// decoded child is rewritten as the union, still marked, and notes the rewrite.
// One that references every child is not written.
//
// Invariant: a short or uncertain list can only delay deletion.
// After a walk finishes with an empty front, every list a root reaches through
// the walker references every child the run decoded from its block's content.
// An undecodable read revokes an earlier mark and returns the list to the front
// and run without dropping any recorded child. Marks and the run's completion
// proof from an earlier ListVerifierRevision are unverified too.
//
// Verify writes the union of the stored list and the refs, since a list may
// lack a child and referencing one more only keeps a block longer. Refs that
// lack a child an unverified stored list references are refused, not written: a
// stored list can only miss children, so such refs come from a wrong decoder,
// and the block stays on the front, which holds the flip. A marked list is past
// the front, so a refusal would leave it lacking with nothing to hold the flip,
// and the sweep would free the child it lacks: it takes the union whatever else
// it references, and MarkedRefused reports the disagreement. A child only a
// wrong decoder references has no marked list, so it joins the front and holds
// the flip. A list that gains a child logs no holder for it, so on a Volume
// that keeps the logs the rewrite records that the counts owe a recount, which
// the flip reads to return the Volume to unadopted first.
//
// The walker reads through the Volume's full read chain, peers included,
// and fills nothing, so a block the Volume lacks, as a peer's cache lacks
// what it never read, is decoded and verified like a stored one: its list
// is written without its bytes, as an evicted block keeps its list, and
// holds the stored blocks below it, which no count held while it had none.
// A block no read resolves is not verified, and stays on the front while
// it has holders, so it holds the flip.
//
// It runs on an unadopted or an adopted Volume, but not while an adoption
// counts, and only while the old generation is Open. OpenListCheck starts a
// dry run, which writes nothing.
type ListRebuild struct {
	// store is the Volume.
	store Store
	// dryRun compares the lists and counts what a rebuild would write.
	dryRun bool
	// pending are the blocks not yet written.
	pending []pendingList
	// res counts the results.
	res ListRebuildResult
}

// pendingList is one block Verify or Undecodable queued.
type pendingList struct {
	// entry holds the block's ref and decoded refs.
	entry *block.PutBatchEntry
	// typ records the block's decoded type, for the samples.
	typ string
	// undecodable reports that the walker could not decode the block, so it
	// is only counted when its list is marked.
	undecodable bool
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

// OpenListCheck starts a dry run of a list rebuild of store, in any state:
// it compares each list with its block's refs in read transactions and
// counts what a rebuild would write, writing nothing.
func OpenListCheck(store Store) *ListRebuild {
	return &ListRebuild{store: store, dryRun: true}
}

// Verify records that ref's decoded refs are refs, writing its list with
// the next batch.
func (r *ListRebuild) Verify(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef) error {
	return r.VerifyAs(ctx, ref, refs, "")
}

// VerifyAs is Verify for a block decoded as the type typ records, which the
// samples carry.
func (r *ListRebuild) VerifyAs(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef, typ string) error {
	return r.queue(ctx, pendingList{entry: &block.PutBatchEntry{Ref: ref, Refs: refs}, typ: typ})
}

// Undecodable records that the walker reached ref, as the type typ records, and
// could not decode it, or not with certainty. The rebuild counts and samples a
// marked list and revokes its mark, preserving its children. A dry run only
// reports the disagreement.
func (r *ListRebuild) Undecodable(ctx context.Context, ref *block.BlockRef, typ string) error {
	return r.queue(ctx, pendingList{entry: &block.PutBatchEntry{Ref: ref}, typ: typ, undecodable: true})
}

// queue adds p to the next batch, flushing a full one.
func (r *ListRebuild) queue(ctx context.Context, p pendingList) error {
	r.pending = append(r.pending, p)
	if len(r.pending) < listRebuildBatch {
		return nil
	}
	return r.flush(ctx)
}

// Flush writes the pending lists, or on a dry run compares them, and returns
// the tally of every block handed to the rebuild so far. A caller that records
// its progress, such as the blocks its walk finished, flushes first, so the
// progress it records covers only lists already written or compared.
func (r *ListRebuild) Flush(ctx context.Context) (ListRebuildResult, error) {
	err := r.flush(ctx)
	return r.res, err
}

// listVerdict is what verifyOne did with one block's decoded refs.
type listVerdict int

const (
	// listVerified is a block whose stored list referenced every child its
	// refs reference.
	listVerified listVerdict = iota
	// listRewritten is a block whose list was missing or lacked a child.
	listRewritten
	// listDropped is a block whose refs lack a child its stored list
	// references.
	listDropped
)

// flush writes the pending lists in one transaction while no adoption
// counts and the generation is Open, or on a dry run compares them in one
// read snapshot, then lets a waiting writer in. The rebuild is upkeep a later
// run redoes, so its transactions take background admission whatever context
// the walker passes, and a foreground writer waits behind at most one batch.
func (r *ListRebuild) flush(ctx context.Context) error {
	// Nothing is pending.
	if len(r.pending) == 0 {
		return nil
	}

	// Compare each marked list on a dry run.
	var res ListRebuildResult
	if r.dryRun {
		err := r.store.View(ctx, func(rec *Records) error {
			res = r.res.samples()
			_, err := r.apply(ctx, rec, &res, false)
			return err
		})
		return r.added(ctx, res, err)
	}

	// Write each list from its refs.
	err := r.store.Update(block.WithBackground(ctx), func(rec *Records, _ block.StoreOps) error {
		// Refuse a Volume an adoption is counting or the settle flipped.
		res = r.res.samples()
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		if err := rebuildable(state); err != nil {
			return err
		}

		// Verify each block, then record a recount the counts owe.
		rewrote, err := r.apply(ctx, rec, &res, true)
		if err != nil || !rewrote {
			return err
		}
		return rec.noteRewrite(ctx, state)
	})
	return r.added(ctx, res, err)
}

// added adds a flushed batch's tally res, unless the batch failed with err,
// and lets a waiting writer in.
func (r *ListRebuild) added(ctx context.Context, res ListRebuildResult, err error) error {
	if err != nil {
		return err
	}
	r.res.add(res)
	r.pending = r.pending[:0]
	return context.Cause(ctx)
}

// apply verifies each pending block in rec, counting into res, writing the
// lists when write is set and otherwise counting what it would write, and
// reports whether a list gained a child.
func (r *ListRebuild) apply(ctx context.Context, rec *Records, res *ListRebuildResult, write bool) (bool, error) {
	var rewrote bool
	for _, p := range r.pending {
		// Read the list, and compare a marked one with the refs.
		key := BlockKey(p.entry.Ref)
		value, listed, err := rec.tx.Get(ctx, rec.key(kindChildren, key))
		if err != nil {
			return false, err
		}
		marked := listed && listMarked(value)
		var lacking bool
		if marked {
			if lacking, err = res.compareMarked(p, value); err != nil {
				return false, err
			}
		}

		// An uncertain read revokes the certificate, never its children.
		if p.undecodable {
			if write && marked {
				children, err := DecodeChildren(value)
				if err != nil {
					return false, err
				}
				if err := rec.putCopied(ctx, p.entry.Ref, children); err != nil {
					return false, err
				}
			}
			continue
		}

		// A current marked list already references every decoded child.
		if marked && !lacking {
			continue
		}

		// Write the list, or on a dry run judge it.
		var verdict listVerdict
		if write {
			verdict, err = verifyOne(ctx, rec, p.entry, value, listed)
		} else {
			verdict, err = judgeList(value, listed, p.entry.Refs)
		}
		if err != nil {
			return false, err
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
			res.DroppedSample = sample(res.DroppedSample, p.entry.Ref)
		}
	}
	return rewrote, nil
}

// judgeList returns what verifyOne would do with a list value, listed or
// not, given its block's decoded refs, writing nothing.
func judgeList(value []byte, listed bool, refs []*block.BlockRef) (listVerdict, error) {
	// A missing list is written from the refs.
	if !listed {
		return listRewritten, nil
	}

	// An unverified one is refused when the refs drop a child it
	// references.
	old, encoded := listKeys(value), EncodeChildren(refs)
	if !listMarked(value) && !containsChildren(encoded, old) {
		return listDropped, nil
	}

	// A listed one is otherwise kept, or rewritten as the union.
	union, err := unionChildren(old, encoded)
	if err != nil || bytes.Equal(union, old) {
		return listVerified, err
	}
	return listRewritten, nil
}

// verifyOne writes the list of the block that entry references from its decoded
// refs, marked, while the generation is Open, given its list value and whether
// it is listed: a missing list from the refs, a listed one, unverified or a
// marked one that lacks a child, as the union, as mergeList describes, strictly
// for an unverified list alone. A missing list is written whether or not the
// Volume stores the bytes, and ends the block's hole record, since the list now
// holds its children. The audit walk counts such a block as ListedAbsent, the
// refill's input. A refused list is left as it was, unverified and on the
// front.
func verifyOne(ctx context.Context, rec *Records, entry *block.PutBatchEntry, value []byte, listed bool) (listVerdict, error) {
	// Write a missing list from the refs, or merge a listed one.
	key := BlockKey(entry.Ref)
	var packed []byte
	verdict := listRewritten
	if !listed {
		packed = EncodeChildren(entry.Refs)
		if err := rec.tx.Set(ctx, rec.key(kindChildren, key), markList(packed)); err != nil {
			return listVerified, err
		}
		if err := rec.tx.Delete(ctx, rec.key(kindHole, key)); err != nil {
			return listVerified, err
		}
	} else {
		var gained, dropped bool
		var err error
		packed, gained, dropped, err = rec.mergeList(ctx, key, value, entry.Refs, !listMarked(value))
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
// as a block the walker could not decode or one whose refs were refused. A
// dry run wrote nothing, so it reports the front as it stood before any
// rebuild, not what one would leave. It counts them in slices that each read
// sliceRecords front entries, so it yields to writers on a live Volume. The
// settle flips once the front is empty and the run is complete.
func (r *ListRebuild) Finish(ctx context.Context) (ListRebuildResult, error) {
	// Write what is pending.
	if err := r.flush(ctx); err != nil {
		return r.res, err
	}

	// Count the front and keep a sample of it.
	r.res.Front, r.res.FrontSample = 0, nil
	err := r.EachFront(ctx, func(ref *block.BlockRef) error {
		r.res.Front++
		if len(r.res.FrontSample) < sampleSize {
			r.res.FrontSample = append(r.res.FrontSample, ref)
		}
		return nil
	})
	return r.res, err
}

// EachFront calls visit with each block on the front, in key order. It reads
// the front in slices of sliceRecords entries and visits each slice once its
// view ends, so it yields to writers on a live Volume and holds one slice.
func (r *ListRebuild) EachFront(ctx context.Context, visit func(ref *block.BlockRef) error) error {
	var from []byte
	for {
		// Read one slice of front keys.
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
			return err
		}

		// Visit the slice outside its view.
		for _, key := range keys {
			ref, _, err := ParseBlockKey(key)
			if err != nil {
				return err
			}
			if err := visit(ref); err != nil {
				return err
			}
		}
		if next == nil {
			return nil
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

// appendGained appends to keys each block key of the encoded child list
// union that the encoded list old, which union contains, lacks. Both hold
// distinct block keys in bytes.Compare order, as EncodeChildren writes them.
func appendGained(keys [][]byte, old, union []byte) [][]byte {
	for len(union) != 0 {
		u, ok := nextBlockKey(union)
		if !ok {
			return keys
		}
		union = union[len(u):]
		if o, ok := nextBlockKey(old); ok && bytes.Equal(o, u) {
			old = old[len(o):]
			continue
		}
		keys = append(keys, u)
	}
	return keys
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
