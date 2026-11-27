package kvtx_block

import (
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
)

// TreeNode is a tree block that holds some of its tree's values by ref. The
// tree's values have no constructor of their own in the node, since the tree
// does not know their type, so a walker that types refs asks the node which
// of its refs are values.
type TreeNode interface {
	// TreeValueRef reports whether the ref with id is one of the tree's
	// values, and whether that value is stored as a Blob.
	TreeValueRef(id uint32) (value, isBlob bool)
}

// TreeValueCtor returns the constructor of the ref with id that the tree
// block blk leaves untyped: a Blob for a value stored as one, values for any
// other value, and nil when blk is no tree node or the ref is no value.
func TreeValueCtor(blk any, id uint32, values block.Ctor) block.Ctor {
	node, ok := blk.(TreeNode)
	if !ok {
		return nil
	}
	value, isBlob := node.TreeValueRef(id)
	switch {
	case !value:
		return nil
	case isBlob:
		return blob.NewBlobBlock
	default:
		return values
	}
}

// Verify the tree nodes implement TreeNode.
var (
	_ TreeNode = (*iavl.Node)(nil)
	_ TreeNode = (*okra.Page)(nil)
)
