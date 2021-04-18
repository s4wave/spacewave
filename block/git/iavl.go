package git

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/iavl"
)

// buildIavlSubBlockTree builds an iavl object tree which applies changes to a sub-block.
func buildIavlSubBlockTree(refID uint32, bcs *block.Cursor, blk block.BlockWithSubBlocks) (*iavl.Tx, error) {
	treeRoot := bcs.FollowSubBlock(refID)
	return iavl.NewTx(treeRoot, true, func(nextRoot *block.Cursor) {
		bcs.SetRef(refID, nextRoot)
		b, _ := nextRoot.GetBlock()
		_ = blk.ApplySubBlock(refID, b)
	})
}
