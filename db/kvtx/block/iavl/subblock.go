package kvtx_block_iavl

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// BuildIavlSubBlockTree builds an iavl object tree which applies changes to a sub-block.
func BuildIavlSubBlockTree(ctx context.Context, refID uint32, bcs *block.Cursor, write bool, blk block.BlockWithSubBlocks) (*Tx, error) {
	treeRoot := bcs.FollowSubBlock(refID)
	return NewTx(ctx, treeRoot, nil, write, func(nextRoot *block.Cursor) {
		bcs.SetRef(refID, nextRoot)
		b, _ := nextRoot.GetBlock()
		subBlk, ok := b.(block.SubBlock)
		if ok {
			_ = blk.ApplySubBlock(refID, subBlk)
		}
	})
}
