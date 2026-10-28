package sobject_world_engine

import (
	"context"
	fmt "fmt"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
)

// retainPublicationWorld fences dependencies for asynchronously persisted providers.
func (c *Controller) retainPublicationWorld(ctx context.Context, so sobject.SharedObject, head *bucket.ObjectRef) error {
	// Skip objects without publication retention or an empty head.
	retention, ok := so.(sobject.PublicationRetention)
	if !ok {
		return nil
	}
	if head.GetRootRef().GetEmpty() {
		return nil
	}

	// Retain the World into the publication's local retention store.
	local, release, err := retention.AccessPublicationRetention(ctx)
	if err != nil {
		return err
	}
	defer release()
	return RetainWorld(ctx, so, head, local, nil)
}

// RetainOptions adjusts a RetainWorld copy.
type RetainOptions struct {
	// Visited observes newly traversed blocks; cached complete subtrees are
	// omitted.
	Visited func(*block.BlockRef, []byte)
	// Missing accepts, when set, a block no reachable store holds or serves as
	// a hole, which leaves the World without a completion proof.
	Missing func(*block.BlockRef)
	// Recheck walks the whole graph, trusting no completion proof, and proves
	// each subtree again. A false proof hides the blocks under it from every
	// other copy.
	Recheck bool
}

// RetainWorld copies a complete World graph into its SharedObject's block store.
// It follows the refs the store reports for each block, so it needs no block
// types. Completion proofs skip immutable subtrees only after their ownership
// writes are durable. The caller owns the proof store and serializes calls for
// it. opts may be nil.
//
// A volume proof does not own its root, so a sweep may remove an unowned
// proved root with its graph at any time. A skipped subtree is held under a
// stage until the copy's writes are fenced, and a proved World counts only if
// its root was still stored when it was written again.
func RetainWorld(
	ctx context.Context,
	so sobject.SharedObject,
	head *bucket.ObjectRef,
	local kvtx.Store,
	opts *RetainOptions,
) error {
	// Fill missing options.
	if opts == nil {
		opts = &RetainOptions{}
	}

	// Short-circuit a World whose graph is already complete in the store. A
	// proof is volume-wide; a second bucket still needs root ownership.
	store := so.GetBlockStore()
	if !opts.Recheck {
		if complete, err := block.RootComplete(ctx, store, head.GetRootRef()); err != nil {
			return err
		} else if complete {
			kept, err := storeRootBlock(ctx, so, head.GetRootRef())
			if err != nil || kept {
				return err
			}
		}
	}

	// Copy the graph through the proof recorder.
	proofs := newRetainProofs(store, store.GetID(), local)
	defer proofs.release()
	copyOpts := &block.GraphCopyOptions{
		Complete: proofs.complete,
		Visited:  opts.Visited,
	}
	if opts.Recheck {
		// Walk the graph streaming, and trust only this copy's own proofs.
		copyOpts.Known = proofs.knownPending
		copyOpts.Forget = true
		copyOpts.Reads = recheckReads
	} else {
		copyOpts.Known = proofs.known
	}
	var holes int
	if opts.Missing != nil {
		copyOpts.Missing = func(_ context.Context, ref *block.BlockRef) error {
			holes++
			opts.Missing(ref)
			return nil
		}
	}
	if err := block.CopyGraph(ctx, store, proofs.writes, head.GetRootRef(), copyOpts); err != nil {
		return err
	}
	if err := proofs.flush(ctx); err != nil {
		return err
	}

	// Mark the root complete unless the copy left a hole.
	if holes != 0 {
		return nil
	}
	return block.MarkRootComplete(ctx, store, head.GetRootRef())
}

// storeRootBlock writes the root block, read locally or fetched on demand, to
// the local store of so, so a named root can hold it. It reports whether the
// root was already stored when written: a root swept since its proof was read
// comes back without the graph under it.
func storeRootBlock(ctx context.Context, so sobject.SharedObject, root *block.BlockRef) (bool, error) {
	// Read the root block with its refs, without filling the store.
	store := so.GetBlockStore()
	var src block.StoreOps = store
	if cs, ok := store.(block.CopySource); ok {
		src = cs.GetCopySource()
	}
	stored, err := src.GetStoredBlock(ctx, root)
	if err != nil {
		return false, err
	}
	if stored == nil {
		return false, fmt.Errorf("retained World root: %w", block.NotFoundError(root))
	}
	if !stored.RefsKnown {
		return false, errors.Wrap(block.ErrRefsUnknown, root.MarshalString())
	}

	// Write it again, noting whether it was still stored.
	entry := &block.PutBatchEntry{Ref: root, Data: stored.Data, Refs: stored.Refs}
	existed, err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{entry})
	if err != nil {
		return false, err
	}
	kept := len(existed) != 0 && existed[0]

	// The caller moves a named root next. A store ordered with the
	// SharedObject state never makes that write durable before this one.
	if sobject.QueueOrdersBlockWrites(so) {
		return kept, nil
	}
	_, err = store.Sync(ctx)
	return kept, err
}

// LocalProofKeyPrefix prefixes the completion proofs RetainWorld keeps in the
// caller's local state store when the block store cannot hold them itself.
const LocalProofKeyPrefix = "world-publication-v3/"

// recheckReads is the number of block reads a recheck keeps in flight. A
// recheck walks every block of a World on a device that is serving it, and
// most of its reads miss the page cache, so it keeps few in flight to leave
// the disk to the World's commits.
const recheckReads = 4

// retainProofBatchEntries is the number of proofs, and of buffered block
// writes, per durability fence.
const retainProofBatchEntries = 1024

// retainProofs records completion proofs for RetainWorld. A proof lands only
// after a Sync fence covers the writes of the subtree it names. Proofs live in
// the volume when the store supports root retention, and otherwise in the
// caller's local state store.
type retainProofs struct {
	store  block.StoreOps
	writes *block.BufferedStore
	// bucketID scopes the local proof keys.
	bucketID string
	local    kvtx.Store
	// volume is set when the store holds proofs itself.
	volume bool
	// stage holds the proved subtrees the copy skips, opened on first use.
	stage block.StoreOps
	// releaseStage releases stage.
	releaseStage func()
	// pending holds proof keys not yet flushed.
	pending map[string]struct{}
	// roots holds volume proofs not yet flushed.
	roots []*block.BlockRef
}

// newRetainProofs builds the proof recorder and the buffered writer it fences.
func newRetainProofs(store block.StoreOps, bucketID string, local kvtx.Store) *retainProofs {
	return &retainProofs{
		store:    store,
		bucketID: bucketID,
		writes: block.NewBufferedStoreWithSettings(context.Background(), store, &block.BufferedStoreSettings{
			MaxPendingEntries: retainProofBatchEntries,
			MaxPendingBytes:   4 << 20,
			DrainBatchEntries: retainProofBatchEntries,
		}),
		local:   local,
		volume:  block.SupportsRootRetention(store),
		pending: make(map[string]struct{}),
	}
}

// key returns the local proof key of ref.
func (p *retainProofs) key(ref *block.BlockRef) string {
	return LocalProofKeyPrefix + p.bucketID + "/" + ref.MarshalString()
}

// known reports which refs have a durable or pending completion proof.
func (p *retainProofs) known(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Start from pending proofs and collect the refs that need checking.
	known := make([]bool, len(refs))
	var check []int
	for i, ref := range refs {
		if _, ok := p.pending[p.key(ref)]; ok {
			known[i] = true
		} else {
			check = append(check, i)
		}
	}
	if len(check) == 0 {
		return known, nil
	}

	// Hold the subtrees with volume proofs until the copy releases them.
	if p.volume {
		return p.hold(ctx, refs, known, check)
	}

	// Read the local proof records for the remaining refs.
	tx, err := p.local.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	var proved []*block.BlockRef
	var provedIdx []int
	for _, i := range check {
		_, found, err := tx.Get(ctx, []byte(p.key(refs[i])))
		if err != nil {
			tx.Discard()
			return nil, err
		}
		if found {
			proved = append(proved, refs[i])
			provedIdx = append(provedIdx, i)
		}
	}
	tx.Discard()
	if len(proved) == 0 {
		return known, nil
	}

	// Collection can invalidate an older completion record. A live parent
	// still retains its descendants through the volume graph.
	exists, err := p.store.GetBlockExistsBatch(ctx, proved)
	if err != nil {
		return nil, err
	}
	for j, i := range provedIdx {
		known[i] = exists[j]
	}
	return known, nil
}

// knownPending reports which refs have a proof this copy recorded and has not
// yet flushed.
func (p *retainProofs) knownPending(_ context.Context, refs []*block.BlockRef) ([]bool, error) {
	known := make([]bool, len(refs))
	for i, ref := range refs {
		_, known[i] = p.pending[p.key(ref)]
	}
	return known, nil
}

// hold reports which checked refs have a volume proof and holds each proved
// subtree under the stage, so it outlives the copy's writes of its parents. A
// proof read first keeps the stage closed for a copy that skips nothing.
func (p *retainProofs) hold(ctx context.Context, refs []*block.BlockRef, known []bool, check []int) ([]bool, error) {
	// Read the proofs of all checked refs in one transaction, and collect the
	// refs whose proof reads present.
	checked := make([]*block.BlockRef, len(check))
	for j, i := range check {
		checked[j] = refs[i]
	}
	complete, err := block.RootsComplete(ctx, p.store, checked)
	if err != nil {
		return nil, err
	}
	var proved []*block.BlockRef
	var provedIdx []int
	for j, i := range check {
		if complete[j] {
			proved = append(proved, refs[i])
			provedIdx = append(provedIdx, i)
		}
	}
	if len(proved) == 0 {
		return known, nil
	}

	// Open the stage, then check the proofs again while it takes the subtrees.
	if p.stage == nil {
		stage, release, err := block.OpenStage(ctx, p.store)
		if err != nil {
			return nil, err
		}
		p.stage, p.releaseStage = stage, release
	}
	held, err := block.HoldCompleteRoots(ctx, p.stage, proved)
	if err != nil {
		return nil, err
	}
	for j, i := range provedIdx {
		known[i] = held[j]
	}
	return known, nil
}

// release drops the stage holding skipped subtrees. The caller has fenced the
// writes that reference them, or abandoned the copy.
func (p *retainProofs) release() {
	if p.releaseStage != nil {
		p.releaseStage()
	}
}

// complete queues a proof for ref and flushes a full batch.
func (p *retainProofs) complete(ctx context.Context, ref *block.BlockRef) error {
	p.pending[p.key(ref)] = struct{}{}
	if p.volume {
		p.roots = append(p.roots, ref)
	}
	if len(p.pending) >= retainProofBatchEntries {
		return p.flush(ctx)
	}
	return nil
}

// flush fences the buffered writes, then records the pending proofs. Failure
// may keep proofs of complete subtrees, but never records a parent whose
// descendants failed.
func (p *retainProofs) flush(ctx context.Context) error {
	// Fence the buffered block writes before recording any proof.
	fenced, err := p.writes.Sync(ctx)
	if err != nil {
		return err
	}
	if !fenced {
		return errors.New("local block store has no durability fence")
	}
	if len(p.pending) == 0 {
		return nil
	}

	// Record volume proofs on the block store itself.
	if p.volume {
		if err := block.MarkRootsComplete(ctx, p.store, p.roots); err != nil {
			return err
		}
		p.roots = nil
		clear(p.pending)
		return nil
	}

	// Record the remaining proofs in the local state store.
	err = kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
		return p.local.NewTransaction(ctx, true)
	}, func(ctx context.Context, tx kvtx.Tx) error {
		for key := range p.pending {
			if err := tx.Set(ctx, []byte(key), []byte{1}); err != nil {
				return err
			}
		}
		return nil
	})
	if err == nil {
		clear(p.pending)
	}
	return err
}
