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
	for {
		done, err := walkBatch(ctx, opts, batch, &progress)
		if err != nil {
			return nil, err
		}
		if done {
			break
		}
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
		added, err := visit(ctx, tx, ref)
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

// visit adds ref to L and the frontier unless L holds it, reporting whether
// it did.
func visit(ctx context.Context, tx kvtx.TxOps, ref *block.BlockRef) (bool, error) {
	// Skip a key L holds.
	tail := RefTail(ref)
	found, err := tx.Exists(ctx, prefixed(visitedPrefix, tail))
	if err != nil || found {
		return false, err
	}

	// Add it to L and the frontier.
	data, err := ref.MarshalVT()
	if err != nil {
		return false, err
	}
	if err := tx.Set(ctx, prefixed(visitedPrefix, tail), nil); err != nil {
		return false, err
	}
	return true, tx.Set(ctx, prefixed(frontierPrefix, tail), data)
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
}

// walkBatch takes up to batch frontier keys, finds their children, and
// commits the children and the holes in place of the keys. It reports done
// when the frontier was empty.
func walkBatch(ctx context.Context, opts *WalkOptions, batch int, progress *WalkProgress) (bool, error) {
	// Read the batch and its refs rows in one snapshot.
	keys, err := readFrontier(ctx, opts.Store, batch)
	if err != nil || len(keys) == 0 {
		return len(keys) == 0, err
	}

	// Ask the source for the keys the catalog cannot serve: a block a
	// backend holds may wait to upload, or have a good copy where every
	// catalogued one is corrupt or lacks its refs.
	var ask []*frontierKey
	for _, k := range keys {
		if k.reason != "" {
			ask = append(ask, k)
		}
	}
	if len(ask) != 0 && opts.Refs != nil {
		refs := make([]*block.BlockRef, len(ask))
		for i, k := range ask {
			refs[i] = k.ref
		}
		results, err := opts.Refs.BlockRefs(ctx, refs)
		if err != nil {
			return false, errors.Wrap(err, "read refs")
		}
		if len(results) != len(ask) {
			return false, errors.Errorf("ref source answered %d of %d blocks", len(results), len(ask))
		}
		for i, k := range ask {
			if results[i].Reason == "" {
				k.refs, k.reason = results[i].Refs, ""
				continue
			}
			k.reason = packcopy.WorseReason(k.reason, results[i].Reason)
		}
	}

	// Open a write with the header.
	tx, err := opts.Store.NewTransaction(ctx, true)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	head, err := readHead(ctx, tx)
	if err != nil {
		return false, err
	}
	if head.Holes == nil {
		head.Holes = make(map[string]uint64)
	}

	// Replace each batch key with its children, or with its hole.
	for _, k := range keys {
		if err := tx.Delete(ctx, prefixed(frontierPrefix, k.tail)); err != nil {
			return false, err
		}
		if k.reason != "" {
			data, err := k.ref.MarshalVT()
			if err != nil {
				return false, err
			}
			if err := tx.Set(ctx, prefixed(holePrefix, k.tail), append([]byte(k.reason+"\x00"), data...)); err != nil {
				return false, err
			}
			head.Holes[k.reason]++
			continue
		}
		for _, child := range k.refs {
			added, err := visit(ctx, tx, child)
			if err != nil {
				return false, err
			}
			if added {
				head.Live++
			}
		}
	}

	// Record the totals and commit.
	if err := writeHead(ctx, tx, head); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	progress.Live = head.Live
	return false, nil
}

// readFrontier reads up to n frontier keys with their catalog refs. A key the
// catalog lacks carries the missing reason until a source answers.
func readFrontier(ctx context.Context, store kvtx.Store, n int) ([]*frontierKey, error) {
	// Open a read.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Take the first n keys.
	var keys []*frontierKey
	errFull := errors.New("full")
	err = tx.ScanPrefix(ctx, []byte(frontierPrefix), func(key, value []byte) error {
		// Decode the key's ref, stopping at the n-th.
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(value); err != nil {
			return errors.Wrap(err, "frontier row")
		}
		keys = append(keys, &frontierKey{tail: bytes.Clone(key[len(frontierPrefix):]), ref: ref})
		if len(keys) == n {
			return errFull
		}
		return nil
	})
	if err != nil && err != errFull {
		return nil, err
	}

	// Read each key's refs from the catalog.
	for _, k := range keys {
		value, found, err := tx.Get(ctx, prefixed(catalogRefsPrefix, k.tail))
		if err != nil {
			return nil, err
		}
		if !found {
			k.reason = packcopy.ReasonMissing
			continue
		}
		state, refs, err := unmarshalRefs(value)
		if err != nil {
			return nil, err
		}
		switch state {
		case refsKnown:
			k.refs = refs
		case refsUnknown:
			k.reason = packcopy.ReasonRefsUnknown
		default:
			k.reason = packcopy.ReasonCorrupt
		}
	}
	return keys, nil
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
