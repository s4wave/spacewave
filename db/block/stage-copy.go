package block

import (
	"context"
	"slices"

	"github.com/pkg/errors"
)

// stageCopyBatchEntries is the number of proofs, and of buffered block
// writes, per durability fence of a StageCopy.
const stageCopyBatchEntries = 1024

// StageCopy copies block graphs into a store under a stage, and skips only
// the subtrees the stage holds. It records a completion proof for each
// subtree it finishes, so the next copy skips it.
//
// Invariants. A copy that skips a block hands it to the parent that names it,
// and the collector frees a stored block that nothing holds, so the copy
// keeps these:
//
//  1. Known reports a block only once the stage holds it: its proof is
//     checked and a stage root moved onto it in one transaction, or the copy
//     itself queued the block's write through the stage, ahead of every
//     parent that names it. A stored block nothing holds is not reported,
//     whether or not it has a proof.
//  2. A proved block's whole subtree is stored: the collector deletes the
//     proof of a block with children in the cascade that releases them, and a
//     leaf's proof with its free. So the stage root on a proved block holds
//     its subtree. A block at zero that the stage root holds is Holding: the
//     hold mark the move sets stops every free until its record nets, and the
//     re-hold then gives its children their holder back.
//  3. A block Known does not report is read and put again through the stage,
//     which holds it from its put. A put of a stored block the collector may
//     have at zero sets the hold mark the same way.
//  4. A proof is recorded only after a durability fence covers the writes of
//     its whole subtree, and never for a block with a hole under it.
//  5. A store that holds no roots, such as a cloud block store, keeps a block
//     because its storage is durable and not because nothing frees it, so the
//     copy skips each block it stores and records no proofs.
//
// The caller keeps the stage open until a root that names the copied roots
// lands, then releases it.
type StageCopy struct {
	// store is the destination, which keeps the proofs.
	store StoreOps
	// staged is the stage the copy writes under.
	staged StoreOps
	// release ends the stage.
	release func()
	// recheck reads every block again, trusting no proof.
	recheck bool
	// writes buffers the copy's writes into staged, or is nil on a store that
	// holds no roots.
	writes *BufferedStore
	// pending holds the keys of the proofs not yet flushed.
	pending map[string]struct{}
	// roots holds the proofs not yet flushed.
	roots []*BlockRef
}

// OpenStageCopy opens a stage on store for a copy. With recheck, the copy
// reads every block again, trusting no proof, and records the proofs of what
// it finds complete. Call Release once the root that names the copied roots
// has landed.
func OpenStageCopy(ctx context.Context, store StoreOps, recheck bool) (*StageCopy, error) {
	// Write the copy under a stage until a root holds it.
	staged, release, err := OpenStage(ctx, store)
	if err != nil {
		return nil, err
	}
	c := &StageCopy{store: store, staged: staged, release: release, recheck: recheck}
	if !HoldsRoots(store) {
		return c, nil
	}

	// Buffer the writes so a fence covers a batch of them.
	c.writes = NewBufferedStoreWithSettings(context.Background(), staged, &BufferedStoreSettings{
		MaxPendingEntries: stageCopyBatchEntries,
		MaxPendingBytes:   4 << 20,
		DrainBatchEntries: stageCopyBatchEntries,
	})
	c.pending = make(map[string]struct{})
	return c, nil
}

// Copy copies the graph under root from src, skipping each subtree the stage
// holds. opts sets the copy's Reads, Visited, Forget and Missing; Copy sets
// its Known and Complete. Call Flush before the root that names root lands.
func (c *StageCopy) Copy(ctx context.Context, src StoreOps, root *BlockRef, opts GraphCopyOptions) error {
	// Copy straight into a store that holds no roots, skipping what it stores.
	opts.Known, opts.Complete = nil, nil
	if c.writes == nil {
		if !c.recheck {
			opts.Known = c.store.GetBlockExistsBatch
		}
		return CopyGraph(ctx, src, c.staged, root, &opts)
	}

	// Copy through the buffer, skipping each subtree the stage holds.
	if !c.recheck {
		opts.Known = c.known
	}
	opts.Complete = c.complete
	return CopyGraph(ctx, src, c.writes, root, &opts)
}

// Flush stores the buffered writes, and records the pending proofs once a
// durability fence covers them. A failure may keep proofs of complete
// subtrees, but never records a parent whose descendants failed.
func (c *StageCopy) Flush(ctx context.Context) error {
	// Store the writes of a copy that proved nothing, which need no fence.
	if c.writes == nil {
		return nil
	}
	if len(c.roots) == 0 {
		return c.writes.Flush(ctx)
	}

	// Fence the buffered block writes before recording any proof.
	fenced, err := c.writes.Sync(ctx)
	if err != nil {
		return err
	}
	if !fenced {
		return errors.New("block store has no durability fence")
	}

	// Record the proofs on the store.
	if err := MarkRootsComplete(ctx, c.store, c.roots); err != nil {
		return err
	}
	c.roots = nil
	clear(c.pending)
	return nil
}

// CheckHeld returns nil when the stage still holds the whole graph under
// each of roots: each has a completion proof, and the open stage holds it in
// the same transaction. Call it after the root that names roots lands and
// before Release. A process that stalled past the stage's lease and the grace
// period may have had the graph freed while nothing wrote through the stage.
// The collector deletes a block's proof before it releases the block's
// children (the netting's release in db/block/gc/netting.go) and with the
// block when it frees it (freeBlock in db/block/gc/collector.go), so a graph
// that lost any block has lost its proof, the check fails with
// ErrStageReleased, and the caller copies again.
func (c *StageCopy) CheckHeld(ctx context.Context, roots ...*BlockRef) error {
	// Hold the roots again under their proofs, which fails on a lost stage.
	if c.writes == nil {
		return nil
	}
	roots = slices.DeleteFunc(slices.Clone(roots), (*BlockRef).GetEmpty)
	if len(roots) == 0 {
		return nil
	}
	held, err := HoldCompleteRoots(ctx, c.staged, roots)
	if err != nil {
		return err
	}
	if i := slices.Index(held, false); i >= 0 {
		return errors.Wrapf(ErrStageReleased, "copied graph %s lost its completion proof", roots[i].MarshalString())
	}
	return nil
}

// Release ends the stage. Every block it held that no root, parent or pin
// reaches is then unheld.
func (c *StageCopy) Release() {
	c.release()
}

// known reports which refs the stage holds, holding each proved subtree under
// it. A proof read first keeps the hold's transaction off a level that has no
// proofs.
func (c *StageCopy) known(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	// Start from the queued writes and read the other proofs in one
	// transaction.
	known := make([]bool, len(refs))
	var check []*BlockRef
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
	complete, err := RootsComplete(ctx, c.store, check)
	if err != nil {
		return nil, err
	}

	// Hold the proved subtrees, checking each proof again in the hold's
	// transaction, since a collection may have freed a block after its read.
	var proved []*BlockRef
	var provedIdx []int
	for j, i := range checkIdx {
		if complete[j] {
			proved, provedIdx = append(proved, refs[i]), append(provedIdx, i)
		}
	}
	if len(proved) == 0 {
		return known, nil
	}
	held, err := HoldCompleteRoots(ctx, c.staged, proved)
	if err != nil {
		return nil, err
	}
	for j, i := range provedIdx {
		known[i] = held[j]
	}
	return known, nil
}

// complete queues the proof of a finished subtree and flushes a full batch.
func (c *StageCopy) complete(ctx context.Context, ref *BlockRef) error {
	c.pending[ref.MarshalString()] = struct{}{}
	c.roots = append(c.roots, ref)
	if len(c.roots) >= stageCopyBatchEntries {
		return c.Flush(ctx)
	}
	return nil
}
