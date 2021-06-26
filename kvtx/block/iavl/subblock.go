package kvtx_block_iavl

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
)

// BuildIavlSubBlockTree builds an iavl object tree which applies changes to a sub-block.
func BuildIavlSubBlockTree(ctx context.Context, refID uint32, bcs *block.Cursor, blk block.BlockWithSubBlocks) (*Tx, error) {
	treeRoot := bcs.FollowSubBlock(refID)
	return NewTx(ctx, treeRoot, true, func(nextRoot *block.Cursor) {
		bcs.SetRef(refID, nextRoot, true)
		b, _ := nextRoot.GetBlock()
		_ = blk.ApplySubBlock(refID, b)
	})
}
