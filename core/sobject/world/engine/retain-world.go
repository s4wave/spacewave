package sobject_world_engine

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx"
)

// retainPublicationWorld fences dependencies for asynchronously persisted providers.
func (c *Controller) retainPublicationWorld(ctx context.Context, so sobject.SharedObject, head *bucket.ObjectRef) error {
	retention, ok := so.(sobject.PublicationRetention)
	if !ok {
		return nil
	}
	if head.GetRootRef().GetEmpty() {
		return nil
	}
	local, release, err := retention.AccessPublicationRetention(ctx)
	if err != nil {
		return err
	}
	defer release()
	return RetainWorld(ctx, so, head, local, nil)
}

// RetainWorld copies a complete World graph into its SharedObject's block store.
// It follows the refs the store reports for each block, so it needs no block
// types. Completion proofs skip immutable subtrees only after their ownership
// writes are durable. The caller owns the proof store and serializes calls for
// it. visited observes newly traversed blocks; cached complete subtrees are
// omitted.
func RetainWorld(ctx context.Context, so sobject.SharedObject, head *bucket.ObjectRef, local kvtx.Store, visited func(*block.BlockRef, []byte)) error {
	store := so.GetBlockStore()
	if complete, err := block.RootComplete(ctx, store, head.GetRootRef()); err != nil {
		return err
	} else if complete {
		// A proof is volume-wide; a second bucket still needs root ownership.
		stored, err := store.GetStoredBlock(ctx, head.GetRootRef())
		if err != nil {
			return err
		}
		if stored == nil {
			return block.ErrNotFound
		}
		entry := &block.PutBatchEntry{Ref: head.GetRootRef(), Data: stored.GetData(), Refs: stored.GetRefs()}
		if err := store.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
			return err
		}
		_, err = store.Sync(ctx)
		return err
	}

	proofs := newRetainProofs(store, store.GetID(), local)
	err := block.CopyGraph(ctx, store, proofs.writes, head.GetRootRef(), &block.GraphCopyOptions{
		Known:    proofs.known,
		Complete: proofs.complete,
		Visited:  visited,
	})
	if err != nil {
		return err
	}
	if err := proofs.flush(ctx); err != nil {
		return err
	}
	return block.MarkRootComplete(ctx, store, head.GetRootRef())
}

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
	return "world-publication-v3/" + p.bucketID + "/" + ref.MarshalString()
}

// known reports which refs have a durable or pending completion proof.
func (p *retainProofs) known(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
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
	if p.volume {
		for _, i := range check {
			var err error
			known[i], err = block.RootComplete(ctx, p.store, refs[i])
			if err != nil {
				return nil, err
			}
		}
		return known, nil
	}

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
	if p.volume {
		if err := block.MarkRootsComplete(ctx, p.store, p.roots); err != nil {
			return err
		}
		p.roots = nil
		clear(p.pending)
		return nil
	}
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
