package provider_local

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// peerDecodedReads asks peers for a block, and when a peer serves one without
// its refs, asks the reader to hand them over once it decodes the block, so
// the block is kept as a peer fill with the refs a write of it records. The
// read path cannot store such a block itself, since the pack would keep it as
// a leaf. Without keeping decoded refs, every read of a block the old owner
// holds without refs, as the old owner holds most of the World, asks the old
// owner again.
type peerDecodedReads struct {
	block.StoreOps
	p *placementState
}

// peerDecoded wraps the peer source of a placement whose peer fills upload.
func (p *placementState) peerDecoded(peers block_store.StoreSource) block_store.StoreSource {
	if !p.peerUpload || peers == nil {
		return peers
	}
	return func() block.StoreOps {
		src := peers()
		if src == nil {
			return nil
		}
		return &peerDecodedReads{StoreOps: src, p: p}
	}
}

// GetStoredBlock reads the block from peers and marks a hit whose refs are
// not asserted to be kept once the reader decodes it: a peer's refs are only
// listed, and a fill of them is dropped where a mark would trust them.
func (r *peerDecodedReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	// Return misses.
	stored, err := r.StoreOps.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return stored, err
	}

	// Fill in the background, bounded as the fallback's fills are, so the
	// reader, which may hold the World's writer, never waits on the local
	// writer.
	p := r.p
	block.KeepDecodedRefs(ctx, ref, stored, func(_ context.Context, kept *block.StoredBlock) {
		p.fallbackFills.start(ref, kept, p.fillFromFallback)
	})
	return stored, nil
}

// GetStoredBlockBatch reads the blocks from peers as one batch and marks
// each hit as GetStoredBlock does.
func (r *peerDecodedReads) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	p := r.p
	return block.GetStoredBlockBatch(ctx, r.StoreOps, refs, func(i int, stored *block.StoredBlock, err error) error {
		if err == nil && stored != nil {
			ref := refs[i]
			block.KeepDecodedRefs(ctx, ref, stored, func(_ context.Context, kept *block.StoredBlock) {
				p.fallbackFills.start(ref, kept, p.fillFromFallback)
			})
		}
		return fn(i, stored, err)
	})
}
