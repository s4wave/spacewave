package provider_local

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// accountWorldRoot is the named root that holds the account World copy.
const accountWorldRoot = "account-world"

// worldCopyBatchEntries is the number of proofs, and of buffered block writes,
// per durability fence.
const worldCopyBatchEntries = 1024

// copyWorldGraph copies the World graph under root into store under a stage,
// then moves the account World root onto it. It skips each subtree whose root
// has a completion proof, holding that subtree under the stage, and marks the
// proof of each subtree it finishes once a fence covers its writes. visited
// observes each block the copy reads; missing, when set, accepts a block no
// source serves as a hole, which leaves its ancestors unproved and the root
// unmoved.
func copyWorldGraph(
	ctx context.Context,
	store block.StoreOps,
	root *block.BlockRef,
	visited func(*block.BlockRef, []byte),
	missing func(*block.BlockRef),
) error {
	// Write the copy under a stage until the root holds it.
	staged, release, err := block.OpenStage(ctx, store)
	if err != nil {
		return err
	}
	defer release()
	c := &worldCopy{
		store:  store,
		staged: staged,
		writes: block.NewBufferedStoreWithSettings(context.Background(), staged, &block.BufferedStoreSettings{
			MaxPendingEntries: worldCopyBatchEntries,
			MaxPendingBytes:   4 << 20,
			DrainBatchEntries: worldCopyBatchEntries,
		}),
		pending: make(map[string]struct{}),
	}

	// Copy the graph unless its root is already proved.
	proved, err := c.known(ctx, []*block.BlockRef{root})
	if err != nil {
		return err
	}
	var holes int
	if !proved[0] {
		opts := &block.GraphCopyOptions{Known: c.known, Complete: c.complete, Visited: visited}
		if missing != nil {
			opts.Missing = func(_ context.Context, ref *block.BlockRef) error {
				holes++
				missing(ref)
				return nil
			}
		}
		if err := block.CopyGraph(ctx, store, c.writes, root, opts); err != nil {
			return err
		}
		if err := c.flush(ctx); err != nil {
			return err
		}
	}

	// Hold the World under its root unless the copy left a hole.
	if holes != 0 {
		return nil
	}
	return holdAccountWorld(ctx, store, root)
}

// worldCopy records the completion proofs of one World copy and holds the
// proved subtrees it skips under its stage.
type worldCopy struct {
	// store is the destination, which keeps the proofs.
	store block.StoreOps
	// staged is the stage the copy writes under.
	staged block.StoreOps
	// writes buffers the copy's writes into staged.
	writes *block.BufferedStore
	// pending holds the keys of the proofs not yet flushed.
	pending map[string]struct{}
	// roots holds the proofs not yet flushed.
	roots []*block.BlockRef
}

// known reports which refs have a proof, holding each proved subtree under the
// stage so it outlives the copy's writes of its parents. A proof read first
// keeps the hold's transaction off a level that has no proofs.
func (c *worldCopy) known(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Start from the pending proofs and read the others in one transaction.
	known := make([]bool, len(refs))
	var check []*block.BlockRef
	var checkIdx []int
	for i, ref := range refs {
		if _, ok := c.pending[ref.MarshalString()]; ok {
			known[i] = true
		} else {
			check, checkIdx = append(check, ref), append(checkIdx, i)
		}
	}
	if len(check) == 0 {
		return known, nil
	}
	complete, err := block.RootsComplete(ctx, c.store, check)
	if err != nil {
		return nil, err
	}

	// Hold the proved subtrees, checking each proof again in the hold's
	// transaction, since a sweep may have taken a root after its read.
	var proved []*block.BlockRef
	var provedIdx []int
	for j, i := range checkIdx {
		if complete[j] {
			proved, provedIdx = append(proved, refs[i]), append(provedIdx, i)
		}
	}
	if len(proved) == 0 {
		return known, nil
	}
	held, err := block.HoldCompleteRoots(ctx, c.staged, proved)
	if err != nil {
		return nil, err
	}
	for j, i := range provedIdx {
		known[i] = held[j]
	}
	return known, nil
}

// complete queues the proof of a finished subtree and flushes a full batch.
func (c *worldCopy) complete(ctx context.Context, ref *block.BlockRef) error {
	c.pending[ref.MarshalString()] = struct{}{}
	c.roots = append(c.roots, ref)
	if len(c.roots) >= worldCopyBatchEntries {
		return c.flush(ctx)
	}
	return nil
}

// flush fences the buffered writes, then records the pending proofs. A failure
// may keep proofs of complete subtrees, but never records a parent whose
// descendants failed.
func (c *worldCopy) flush(ctx context.Context) error {
	// Fence the buffered block writes before recording any proof.
	fenced, err := c.writes.Sync(ctx)
	if err != nil {
		return err
	}
	if !fenced {
		return errors.New("local block store has no durability fence")
	}

	// Record the proofs on the store.
	if err := block.MarkRootsComplete(ctx, c.store, c.roots); err != nil {
		return err
	}
	c.roots = nil
	clear(c.pending)
	return nil
}

// holdAccountWorld moves the account World root onto the copied World. The
// copy routine is the root's only writer, so it reads the root it moves from;
// a move that loses a race returns ErrRootMoved and the next copy tries again.
// A store that holds no roots keeps every block it stores, so it needs no
// hold.
func holdAccountWorld(ctx context.Context, store block.StoreOps, root *block.BlockRef) error {
	// Read what the root holds.
	held, err := store.GetRoot(ctx, accountWorldRoot)
	if err != nil {
		return err
	}

	// Move it onto the copy.
	err = store.SetRoots(ctx, nil, []block.NamedRoot{{Name: accountWorldRoot, Ref: root, Previous: held}})
	if errors.Is(err, block.ErrRootsUnheld) {
		return nil
	}
	return err
}
