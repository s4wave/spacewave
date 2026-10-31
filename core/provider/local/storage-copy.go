package provider_local

import (
	"bytes"
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// CopyToBackend copies every block the account's volume stores to the open
// storage backend of a SharedObject's block store, until the copy completes or
// ctx ends, following the store across remounts. The copy resumes where the
// last one to the same backend stopped and reports its progress in the upload
// status. It returns at once when a copy to the backend already completed.
//
// The volume shares values across its block stores, so the copy writes the
// blocks of every store in it to this store's backend. The blocks of the other
// stores are sealed with their own keys, so this store never reads them.
func (a *ProviderAccount) CopyToBackend(ctx context.Context, sharedObjectID string, opts block_store_writeback.CopyOptions) error {
	// Retain the shared object's block-store tracker.
	blockStoreID := a.lookupSharedObjectBlockStoreID(sharedObjectID)
	if blockStoreID == "" {
		return sobject.ErrSharedObjectNotFound
	}
	vol, ok := a.GetVolume().(volume_kvtx.KvtxVolume)
	if !ok {
		return errors.New("volume has no key-value store to list blocks from")
	}
	tkrRef, tkr, _ := a.bstores.AddKeyRef(blockStoreID)
	defer tkrRef.Release()

	// Copy through each mounted handle until one finishes.
	for {
		bs, err := tkr.bstoreCtr.WaitValue(ctx, nil)
		if err != nil {
			return err
		}
		handleCtx, handleCancel := context.WithCancel(ctx)
		go func() {
			_, _ = tkr.bstoreCtr.WaitValueChange(handleCtx, bs, nil)
			handleCancel()
		}()
		err = bs.copyToBackend(handleCtx, vol, opts)
		handleCancel()
		if ctx.Err() != nil {
			return context.Canceled
		}
		if err == nil || (!errors.Is(err, context.Canceled) && handleCtx.Err() == nil) {
			return err
		}
	}
}

// copyToBackend copies the volume's blocks to the store's open backend,
// waiting for one to open.
func (b *BlockStore) copyToBackend(ctx context.Context, vol volume_kvtx.KvtxVolume, opts block_store_writeback.CopyOptions) error {
	// Wait for an open backend.
	var remote *openBackend
	for {
		_, changed := b.placement.wb.GetStatus()
		if remote = b.placement.remote.Load(); remote != nil {
			break
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-changed:
		}
	}

	// Copy the blocks in key order.
	list := listVolumeBlocks(vol)
	return b.placement.wb.Copy(ctx, remote.backend.GetId(), remote.store, list, opts)
}

// listVolumeBlocks lists the blocks the volume stores in key order, each call
// in its own read transaction, so the walk pins no freed space between calls.
// A list position is the block's key past the block prefix: its ref.
func listVolumeBlocks(vol volume_kvtx.KvtxVolume) block_store_writeback.CopyList {
	prefix := vol.GetKvKey().GetBlockFullPrefix()
	return func(ctx context.Context, after []byte, limit int) ([]*block.BlockRef, []byte, error) {
		// Open a short read and position it past after.
		tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
		if err != nil {
			return nil, nil, err
		}
		defer tx.Discard()
		it := tx.Iterate(ctx, prefix, true, false)
		defer it.Close()
		if after != nil {
			if err := it.Seek(bytes.Join([][]byte{prefix, after, {0}}, nil)); err != nil {
				return nil, nil, err
			}
		} else {
			it.Next()
		}

		// Decode up to limit refs from the keys, then report whether more
		// follow.
		var refs []*block.BlockRef
		var last []byte
		for ; it.Valid() && len(refs) < limit; it.Next() {
			last = bytes.Clone(it.Key()[len(prefix):])
			ref := &block.BlockRef{}
			if err := ref.UnmarshalVT(bytes.Clone(last)); err != nil {
				return nil, nil, errors.Wrap(err, "decode stored block key")
			}
			refs = append(refs, ref)
		}
		if err := it.Err(); err != nil {
			return nil, nil, err
		}
		if !it.Valid() {
			return refs, nil, nil
		}
		return refs, last, nil
	}
}
