package packsweep

import (
	"bytes"
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/packcopy"
	"github.com/s4wave/spacewave/db/kvtx"
)

// DefaultWalkBatch is the frontier keys one walk transaction takes when the
// options set none.
const DefaultWalkBatch = 4096

// RefsResult is what a RefSource knows of one block.
type RefsResult struct {
	// Refs are the block's children when Reason is empty.
	Refs []*block.BlockRef
	// Reason is the packcopy hole reason when the source cannot serve the
	// block's refs, empty when it can.
	Reason string
}

// RefSource serves the child lists of blocks no catalogued pack holds, such as
// blocks a backend holds that wait to upload.
type RefSource interface {
	// BlockRefs returns what the source knows of each ref, in order.
	BlockRefs(ctx context.Context, refs []*block.BlockRef) ([]RefsResult, error)
}

// SourceRefs serves refs from packcopy sources, tried in order, checking each
// block's bytes against its ref as a copy does.
type SourceRefs []packcopy.Source

// BlockRefs reads each block from the first source that serves it.
func (s SourceRefs) BlockRefs(ctx context.Context, refs []*block.BlockRef) ([]RefsResult, error) {
	out := make([]RefsResult, len(refs))
	for i, ref := range refs {
		out[i].Reason = packcopy.ReasonMissing
		for _, src := range s {
			stored, reason, err := packcopy.ReadVerified(ctx, src, ref)
			if err != nil {
				return nil, err
			}
			if reason == "" {
				out[i] = RefsResult{Refs: nonEmpty(stored.Refs)}
				break
			}
			out[i].Reason = packcopy.WorseReason(out[i].Reason, reason)
		}
	}
	return out, nil
}

// WalkOptions configures Walk.
type WalkOptions struct {
	// Store is the scratch store, holding a complete snapshot.
	Store kvtx.Store
	// Roots are the blocks every live block is reached from. A resumed walk
	// must pass the same roots.
	Roots []*block.BlockRef
	// LogPosition is the write log's position at the roots, empty for none.
	LogPosition string
	// Refs serves blocks the catalog lacks; nil makes each a missing hole.
	Refs RefSource
	// Batch is the frontier keys per transaction. Zero uses
	// DefaultWalkBatch.
	Batch int
	// Progress observes the walk after each batch, nil for none.
	Progress func(WalkProgress)
	// afterBatch is a test hook run after each batch commits; an error stops
	// the walk there.
	afterBatch func() error
}

// WalkProgress reports a walk.
type WalkProgress struct {
	// Live is the keys in L so far.
	Live uint64
	// Frontier is the keys waiting.
	Frontier uint64
	// Batches is the batches this run committed.
	Batches int
}

// Walk walks what the roots reach into L, from the catalog's refs, listing
// the holes. It commits a batch of the frontier at a time, so a walk killed
// at any point resumes from its last batch to the same L.
func Walk(ctx context.Context, opts *WalkOptions) (*Head, error) {
	// Refuse a walk before the snapshot is complete.
	head, err := ReadHead(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	if !head.Catalogued {
		return nil, errors.New("walk needs a complete snapshot")
	}

	// Name the roots in order, without repeats or empty refs.
	roots := make([]string, 0, len(opts.Roots))
	for _, root := range opts.Roots {
		if !root.GetEmpty() {
			roots = append(roots, root.MarshalString())
		}
	}
	slices.Sort(roots)
	roots = slices.Compact(roots)

	// Refuse other roots on a resumed walk, and seed the frontier once.
	if head.Seeded && !slices.Equal(head.Roots, roots) {
		return nil, errors.New("walk resumed with roots other than the cycle's")
	}
	if head.Walked {
		return head, nil
	}
	if !head.Seeded {
		if head, err = seed(ctx, opts, roots); err != nil {
			return nil, err
		}
	}

	// Walk a batch at a time until the frontier is empty.
	batch := opts.Batch
	if batch <= 0 {
		batch = DefaultWalkBatch
	}
	progress := WalkProgress{Live: head.Live}
	w := &walker{store: opts.Store, refs: opts.Refs, set: liveSet}
	for {
		done, live, err := w.batch(ctx, batch)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
		progress.Live = live
		progress.Batches++
		if opts.Progress != nil {
			opts.Progress(progress)
		}
		if opts.afterBatch != nil {
			if err := opts.afterBatch(); err != nil {
				return nil, err
			}
		}
	}
	return updateHead(ctx, opts.Store, func(h *Head) error {
		h.Walked = true
		return nil
	})
}

// seed puts the roots on the frontier and in L, and records them, in one
// transaction.
func seed(ctx context.Context, opts *WalkOptions, roots []string) (*Head, error) {
	// Open a write with the header.
	tx, err := opts.Store.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	head, err := readHead(ctx, tx)
	if err != nil {
		return nil, err
	}

	// Visit each root.
	for _, s := range roots {
		ref, err := block.UnmarshalBlockRefB58(s)
		if err != nil {
			return nil, err
		}
		added, err := visit(ctx, tx, liveSet, ref)
		if err != nil {
			return nil, err
		}
		if added {
			head.Live++
		}
	}

	// Record the roots and commit.
	head.Roots = roots
	head.LogPosition = opts.LogPosition
	head.Seeded = true
	if err := writeHead(ctx, tx, head); err != nil {
		return nil, err
	}
	return head, tx.Commit(ctx)
}

// walkSet names the rows of one walked set of keys: L, or the closure W.
type walkSet struct {
	// visited prefixes the set's keys.
	visited string
	// frontier prefixes the set's keys whose children wait.
	frontier string
	// stop is the visited prefix of a set whose keys this one does not take,
	// empty for none: W takes no key of L, whose children L holds.
	stop string
	// count returns the head's count of the keys the set's walk added.
	count func(*Head) *uint64
}

// liveSet is L, what the roots reach.
var liveSet = walkSet{
	visited:  visitedPrefix,
	frontier: frontierPrefix,
	count:    func(h *Head) *uint64 { return &h.Live },
}

// closureSet is W, what the log since R reaches that L lacks.
var closureSet = walkSet{
	visited:  closureVisitedPrefix,
	frontier: closureFrontierPrefix,
	stop:     visitedPrefix,
	count:    func(h *Head) *uint64 { return &h.Closure },
}

// visit adds ref to the set and its frontier unless the set or its stop set
// holds it, reporting whether it did.
func visit(ctx context.Context, tx kvtx.TxOps, set walkSet, ref *block.BlockRef) (bool, error) {
	// Skip a key the set or its stop set holds.
	tail := RefTail(ref)
	for _, prefix := range [2]string{set.stop, set.visited} {
		if prefix == "" {
			continue
		}
		found, err := tx.Exists(ctx, prefixed(prefix, tail))
		if err != nil || found {
			return false, err
		}
	}

	// Add it to the set and the frontier.
	data, err := ref.MarshalVT()
	if err != nil {
		return false, err
	}
	if err := tx.Set(ctx, prefixed(set.visited, tail), nil); err != nil {
		return false, err
	}
	return true, tx.Set(ctx, prefixed(set.frontier, tail), data)
}

// walker walks one set a batch of its frontier at a time.
type walker struct {
	// store is the scratch store.
	store kvtx.Store
	// refs serves blocks the catalog lacks, nil for none.
	refs RefSource
	// set is the walked set.
	set walkSet
}

// frontierKey is one key taken from the frontier.
type frontierKey struct {
	// tail is its tail.
	tail []byte
	// ref is its ref.
	ref *block.BlockRef
	// refs are its children, once known.
	refs []*block.BlockRef
	// reason is its hole reason, empty for none.
	reason string
	// skip is set for a key the stop set holds, whose children it holds.
	skip bool
}

// batch takes up to n frontier keys, finds their children, and commits the
// children and the holes in place of the keys. It reports done when the
// frontier was empty, and the set's count after the batch.
func (w *walker) batch(ctx context.Context, n int) (bool, uint64, error) {
	// Read the batch and its refs rows in one snapshot.
	keys, err := w.readFrontier(ctx, n)
	if err != nil || len(keys) == 0 {
		return len(keys) == 0, 0, err
	}

	// Ask the source for the keys the catalog cannot serve: a block a
	// backend holds may wait to upload, or have a good copy where every
	// catalogued one is corrupt or lacks its refs.
	if err := w.askRefs(ctx, keys); err != nil {
		return false, 0, err
	}
	return w.commit(ctx, keys)
}

// askRefs asks the ref source for the keys with a hole reason, keeping the
// worse reason of a key it cannot serve either.
func (w *walker) askRefs(ctx context.Context, keys []*frontierKey) error {
	// Collect the keys to ask for.
	var ask []*frontierKey
	for _, k := range keys {
		if k.reason != "" {
			ask = append(ask, k)
		}
	}
	if len(ask) == 0 || w.refs == nil {
		return nil
	}

	// Ask, and take each answer.
	refs := make([]*block.BlockRef, len(ask))
	for i, k := range ask {
		refs[i] = k.ref
	}
	results, err := w.refs.BlockRefs(ctx, refs)
	if err != nil {
		return errors.Wrap(err, "read refs")
	}
	if len(results) != len(ask) {
		return errors.Errorf("ref source answered %d of %d blocks", len(results), len(ask))
	}
	for i, k := range ask {
		if results[i].Reason == "" {
			k.refs, k.reason = results[i].Refs, ""
			continue
		}
		k.reason = packcopy.WorseReason(k.reason, results[i].Reason)
	}
	return nil
}

// commit replaces each batch key with its children, or its hole, in one
// transaction, and returns the set's count after it.
func (w *walker) commit(ctx context.Context, keys []*frontierKey) (bool, uint64, error) {
	// Open a write with the header.
	tx, err := w.store.NewTransaction(ctx, true)
	if err != nil {
		return false, 0, err
	}
	defer tx.Discard()
	head, err := readHead(ctx, tx)
	if err != nil {
		return false, 0, err
	}
	if head.Holes == nil {
		head.Holes = make(map[string]uint64)
	}
	count := w.set.count(head)

	// Replace each batch key with its children, or with its hole; a key the
	// stop set holds just leaves the frontier.
	for _, k := range keys {
		if err := tx.Delete(ctx, prefixed(w.set.frontier, k.tail)); err != nil {
			return false, 0, err
		}
		if k.skip {
			continue
		}
		if k.reason != "" {
			if err := writeHole(ctx, tx, head, k); err != nil {
				return false, 0, err
			}
			continue
		}
		for _, child := range k.refs {
			added, err := visit(ctx, tx, w.set, child)
			if err != nil {
				return false, 0, err
			}
			if added {
				*count++
			}
		}
	}

	// Record the totals and commit.
	if err := writeHead(ctx, tx, head); err != nil {
		return false, 0, err
	}
	return false, *count, tx.Commit(ctx)
}

// writeHole records a key's hole and counts it in head. L and W share the
// hole rows: W takes no key of L, so a key is a hole of one set at most.
func writeHole(ctx context.Context, tx kvtx.TxOps, head *Head, k *frontierKey) error {
	// Write the row with its reason, and count it.
	data, err := k.ref.MarshalVT()
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, prefixed(holePrefix, k.tail), append([]byte(k.reason+"\x00"), data...)); err != nil {
		return err
	}
	head.Holes[k.reason]++
	return nil
}

// readFrontier reads up to n frontier keys with their catalog refs. A key the
// catalog lacks carries the missing reason until a source answers; a key the
// stop set holds is marked to skip.
func (w *walker) readFrontier(ctx context.Context, n int) ([]*frontierKey, error) {
	// Open a read.
	tx, err := w.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Take the first n keys.
	var keys []*frontierKey
	errFull := errors.New("full")
	prefix := []byte(w.set.frontier)
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		// Decode the key's ref, stopping at the n-th.
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(value); err != nil {
			return errors.Wrap(err, "frontier row")
		}
		keys = append(keys, &frontierKey{tail: bytes.Clone(key[len(prefix):]), ref: ref})
		if len(keys) == n {
			return errFull
		}
		return nil
	})
	if err != nil && err != errFull {
		return nil, err
	}

	// Read each key's refs from the catalog, unless the stop set holds it.
	for _, k := range keys {
		if w.set.stop != "" {
			if k.skip, err = tx.Exists(ctx, prefixed(w.set.stop, k.tail)); err != nil {
				return nil, err
			}
		}
		if !k.skip {
			if err := readCatalogRefs(ctx, tx, k); err != nil {
				return nil, err
			}
		}
	}
	return keys, nil
}

// readCatalogRefs sets a key's refs from the catalog, or its hole reason.
func readCatalogRefs(ctx context.Context, tx kvtx.TxOps, k *frontierKey) error {
	// Read the refs row; none is a missing key.
	value, found, err := tx.Get(ctx, prefixed(catalogRefsPrefix, k.tail))
	if err != nil {
		return err
	}
	if !found {
		k.reason = packcopy.ReasonMissing
		return nil
	}

	// Decode it.
	state, refs, err := unmarshalRefs(value)
	if err != nil {
		return err
	}
	switch state {
	case refsKnown:
		k.refs = refs
	case refsUnknown:
		k.reason = packcopy.ReasonRefsUnknown
	default:
		k.reason = packcopy.ReasonCorrupt
	}
	return nil
}

// Hole is a reached block the walk could not see under.
type Hole struct {
	// Ref is the block.
	Ref *block.BlockRef
	// Reason is its packcopy hole reason.
	Reason string
}

// Holes calls fn with each hole of the cycle's walk.
func Holes(ctx context.Context, store kvtx.Store, fn func(Hole) error) error {
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return tx.ScanPrefix(ctx, []byte(holePrefix), func(_, value []byte) error {
		// Decode the reason and the ref.
		reason, data, ok := bytes.Cut(value, []byte{0})
		if !ok {
			return errors.New("malformed hole row")
		}
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(data); err != nil {
			return errors.Wrap(err, "hole row")
		}
		return fn(Hole{Ref: ref, Reason: string(reason)})
	})
}

// BlockingHoles returns the holes by reason that stop a cycle from removing
// anything: every reason but missing, and missing too unless accepted lists
// each missing block by its b58 ref.
func BlockingHoles(head *Head, accepted map[string]bool, holes []Hole) map[string]uint64 {
	// Count every reason but missing.
	out := make(map[string]uint64)
	for reason, n := range head.Holes {
		if reason != packcopy.ReasonMissing && n != 0 {
			out[reason] = n
		}
	}

	// Count the missing blocks the operator did not accept.
	for _, h := range holes {
		if h.Reason == packcopy.ReasonMissing && !accepted[h.Ref.MarshalString()] {
			out[packcopy.ReasonMissing]++
		}
	}
	maps.DeleteFunc(out, func(_ string, n uint64) bool { return n == 0 })
	return out
}
