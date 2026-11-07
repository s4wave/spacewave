package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// windowFill is where a placed store's bucket reads write the blocks of each
// window they fetch.
type windowFill struct {
	// ctx scopes the writes: the mount's context.
	ctx context.Context
	// target fills the local store.
	target block.StoreOps
}

// windowFillOps fills the local store with the blocks of a fetched pack
// window, as a backend hit fills it, and skips the blocks it holds, such as
// the block the read asked for, which the read fills itself.
type windowFillOps struct {
	// StoreOps is the local store.
	block.StoreOps
	// fill writes one block read from the backend, as placementState.fill.
	fill func(ctx context.Context, primary block.StoreOps, ref *block.BlockRef, stored *block.StoredBlock) error
}

// PutBlockBatch fills each block the local store lacks.
func (w *windowFillOps) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Skip the blocks the local store holds.
	refs := make([]*block.BlockRef, len(entries))
	for i, e := range entries {
		refs[i] = e.Ref
	}
	have, err := w.StoreOps.GetBlockExistsBatch(ctx, refs)
	if err != nil {
		return nil, err
	}

	// Fill the rest.
	for i, e := range entries {
		if have[i] || e.Tombstone {
			continue
		}
		stored := &block.StoredBlock{Data: e.Data, Refs: e.Refs, RefsKnown: e.RefsKnown}
		if err := w.fill(ctx, w.StoreOps, e.Ref, stored); err != nil {
			return nil, err
		}
	}
	return have, nil
}

// fillWindows makes store's pack reads fill the local store with each window
// they fetch, while the mount fills its local store from the backend.
func (t *bstoreTracker) fillWindows(store backendStore) {
	packs, ok := store.(*block_store_s3.PackStore)
	if !ok {
		return
	}
	if wf := t.windowFill.Load(); wf != nil {
		packs.SetWindowFill(wf.ctx, wf.target)
	}
}
