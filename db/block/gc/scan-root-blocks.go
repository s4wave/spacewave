package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// ScanRootBlocks streams the block seeds of the collector's own root scan in
// one snapshot, including legacy roots and every non-block owner's edges.
// It does not walk child lists or build a reached set. The caller deduplicates
// seeds, since several owners may retain the same block.
func (g *RefGraph) ScanRootBlocks(ctx context.Context, yield func(owner string, ref *block.BlockRef) error) error {
	return g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		var stopped error
		err := seedMarkTx(ctx, tx, newBlockMark(nil), func(node, namer string) {
			if ref, ok := ParseBlockIRI(node); ok && stopped == nil {
				stopped = yield(namer, ref)
			}
		})
		if err != nil {
			return err
		}
		return stopped
	})
}
