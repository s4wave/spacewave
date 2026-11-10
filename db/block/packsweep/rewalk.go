package packsweep

import (
	"bytes"
	"context"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// RewalkOptions configures Rewalk.
type RewalkOptions struct {
	// Store is the scratch store, holding a complete snapshot. Rewalk only
	// reads it.
	Store kvtx.Store
	// Holes are the blocks an earlier walk could not see under, by full ref:
	// the lines of its holes file, or its hole rows.
	Holes []*block.BlockRef
	// Refs serves blocks the catalog lacks; nil makes each a missing hole.
	Refs RefSource
	// Batch is the keys per catalog read. Zero uses DefaultWalkBatch.
	Batch int
	// Progress observes the re-walk after each batch, nil for none.
	Progress func(RewalkProgress)
}

// RewalkProgress reports a re-walk.
type RewalkProgress struct {
	// Marked is the keys reached so far, the old holes included.
	Marked uint64
	// Frontier is the keys waiting.
	Frontier int
	// Holes is the holes found so far.
	Holes int
	// Batches is the batches read.
	Batches int
}

// RewalkResult is what a re-walk found.
type RewalkResult struct {
	// Marked is the keys reached from the old holes, the old holes included.
	Marked uint64
	// Filled is the old holes whose refs the catalog or the source now
	// serves.
	Filled int
	// Holes are the blocks the re-walk could not see under, in the order of
	// the hole rows: the old holes still missing, then the keys newly
	// reached that the catalog lacks.
	Holes []Hole
}

// Counts returns the result's holes by reason.
func (r *RewalkResult) Counts() map[string]uint64 {
	out := make(map[string]uint64)
	for _, h := range r.Holes {
		out[h.Reason]++
	}
	return out
}

// Rewalk walks from the holes of an earlier walk through the catalog, which a
// later snapshot may have filled, listing the holes below them. An old hole
// the catalog now holds has its subtree walked; one it still lacks stays a
// hole; a key the subtree reaches that the catalog lacks is a new hole.
//
// It writes nothing: the reached set lives in memory for the call and the
// cycle's walk state is untouched, so the result lists holes and proves no
// pack dead. A key two old holes reach is walked once.
func Rewalk(ctx context.Context, opts *RewalkOptions) (*RewalkResult, error) {
	// Refuse a re-walk before the snapshot is complete.
	head, err := ReadHead(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	if !head.Catalogued {
		return nil, errors.New("re-walk needs a complete snapshot")
	}

	// Seed the frontier with each old hole once.
	res := &RewalkResult{}
	marked := make(map[string]struct{})
	var frontier []*frontierKey
	mark := func(ref *block.BlockRef) {
		// Skip an empty ref and a key already marked.
		if ref.GetEmpty() {
			return
		}
		tail := RefTail(ref)
		if _, ok := marked[string(tail)]; ok {
			return
		}

		// Mark it and put it on the frontier.
		marked[string(tail)] = struct{}{}
		frontier = append(frontier, &frontierKey{tail: tail, ref: ref})
	}
	for _, ref := range opts.Holes {
		mark(ref)
	}
	seeds := len(frontier)

	// Walk a batch at a time until the frontier is empty, counting the
	// seeds the catalog or the source now serves.
	batch := opts.Batch
	if batch <= 0 {
		batch = DefaultWalkBatch
	}
	w := &walker{refs: opts.Refs}
	var progress RewalkProgress
	for taken := 0; len(frontier) != 0; {
		// Take a batch and find its refs.
		n := min(batch, len(frontier))
		keys := frontier[:n:n]
		frontier = frontier[n:]
		if err := readCatalogBatch(ctx, opts.Store, keys); err != nil {
			return nil, err
		}
		if err := w.askRefs(ctx, keys); err != nil {
			return nil, err
		}

		// Record each key's hole, or mark its children.
		for i, k := range keys {
			if k.reason != "" {
				res.Holes = append(res.Holes, Hole{Ref: k.ref, Reason: k.reason})
				continue
			}
			if taken+i < seeds {
				res.Filled++
			}
			for _, child := range k.refs {
				mark(child)
			}
		}
		taken += n

		// Report the batch.
		progress.Marked, progress.Frontier, progress.Holes = uint64(len(marked)), len(frontier), len(res.Holes)
		progress.Batches++
		if opts.Progress != nil {
			opts.Progress(progress)
		}
	}

	// Order the holes as the hole rows are, by tail.
	res.Marked = uint64(len(marked))
	slices.SortFunc(res.Holes, func(a, b Hole) int {
		return bytes.Compare(RefTail(a.Ref), RefTail(b.Ref))
	})
	return res, nil
}

// readCatalogBatch reads each key's refs from the catalog in one snapshot.
func readCatalogBatch(ctx context.Context, store kvtx.Store, keys []*frontierKey) error {
	// Open a read.
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Read each key's refs row, or its hole reason.
	for _, k := range keys {
		k.refs, k.reason = nil, ""
		if err := readCatalogRefs(ctx, tx, k); err != nil {
			return err
		}
	}
	return nil
}
