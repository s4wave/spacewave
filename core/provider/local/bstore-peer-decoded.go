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
// a leaf. Without this, every read of a block the old owner holds without
// refs, as it holds most of the World, asks the old owner again.
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

// GetStoredBlock reads the block from peers and marks a hit without refs to
// be kept once the reader decodes it.
func (r *peerDecodedReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	stored, err := r.StoreOps.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil || stored.RefsKnown {
		return stored, err
	}

	// Fill in the background, bounded as the fallback's fills are, so the
	// reader, which may hold the World's writer, never waits on the local
	// writer.
	p := r.p
	block.KeepDecodedRefs(ctx, ref, stored.Data, func(ctx context.Context, data []byte, refs []*block.BlockRef) {
		p.fallbackFills.start(ctx, ref, &block.StoredBlock{Data: data, Refs: refs, RefsKnown: true}, p.fillFromFallback)
	})
	return stored, nil
}
