package iavl

import (
	"github.com/aperturerobotics/hydra/block"
)

// BuildIavlSubBlockTree builds an iavl object tree which applies changes to a sub-block.
func BuildIavlSubBlockTree(refID uint32, bcs *block.Cursor, blk block.BlockWithSubBlocks) (*Tx, error) {
	treeRoot := bcs.FollowSubBlock(refID)
	return NewTx(treeRoot, true, func(nextRoot *block.Cursor) {
		bcs.SetRef(refID, nextRoot)
		b, _ := nextRoot.GetBlock()
		_ = blk.ApplySubBlock(refID, b)
	})
}
