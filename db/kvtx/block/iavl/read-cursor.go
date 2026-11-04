package kvtx_block_iavl

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// findReadLeaf follows stored immutable descendants beneath the transaction's
// inline root. The root's two bindings stay private to this readonly revision.
func (t *Tx) findReadLeaf(ctx context.Context, key []byte) (*Node, block.ReadCursor, error) {
	// Select the root branch without rebuilding mutable parent edges.
	if err := ctx.Err(); err != nil {
		return nil, block.ReadCursor{}, err
	}
	branch := 1
	if bytes.Compare(key, t.root.GetKey()) < 0 {
		branch = 0
	}
	cursor := *t.readBranches[branch]
	node, err := block.UnmarshalReadBlock[Node](ctx, cursor)
	if err != nil {
		return nil, cursor, err
	}
	if err := node.Validate(); err != nil {
		return nil, cursor, err
	}

	// Search only the stored branch containing this key.
	for !node.IsLeaf() {
		id, ref := uint32(6), node.GetRightChildRef()
		if bytes.Compare(key, node.GetKey()) < 0 {
			id, ref = 5, node.GetLeftChildRef()
		}
		cursor = cursor.FollowRef(id, ref)
		node, err = block.UnmarshalReadBlock[Node](ctx, cursor)
		if err != nil {
			return nil, cursor, err
		}
		if err := node.Validate(); err != nil {
			return nil, cursor, err
		}
	}
	if !bytes.Equal(node.GetKey(), key) {
		return nil, cursor, nil
	}
	return node, cursor, nil
}

// readLeafCursor materializes only the matching leaf for the existing value
// and Blob APIs, keeping their sub-block and ownership rules unchanged.
func readLeafCursor(ctx context.Context, cursor block.ReadCursor) (*block.Cursor, error) {
	leaf := cursor.Cursor()
	if _, err := block.UnmarshalBlock[Node](ctx, leaf); err != nil {
		return nil, err
	}
	return leaf, nil
}
