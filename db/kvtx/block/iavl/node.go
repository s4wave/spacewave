package kvtx_block_iavl

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"gonum.org/v1/gonum/graph/encoding"
)

// NewNodeBlock constructs a new node block.
func NewNodeBlock() block.Block {
	return &Node{}
}

// loadNode follows the node cursor.
// may return nil
func loadNode(ctx context.Context, cursor *block.Cursor) (*Node, error) {
	ni, err := cursor.Unmarshal(ctx, func() block.Block { return &Node{} })
	if err != nil {
		return nil, err
	}
	niv, ok := ni.(*Node)
	if !ok || niv == nil {
		return nil, nil
	}
	if err := niv.Validate(); err != nil {
		return nil, err
	}
	return niv, nil
}

// IsNil checks if the object is nil.
func (n *Node) IsNil() bool {
	return n == nil
}

// ValueIsBlob checks if the value is a blob.
func (n *Node) ValueIsBlob() bool {
	return n.GetValueBlob().GetTotalSize() != 0
}

// Validate does cursory checks on the node.
func (n *Node) Validate() error {
	// enforce empty non-leaf nodes
	if n.GetHeight() != 0 {
		if n.GetValueRef().SizeVT() != 0 {
			return ErrUnexpectedValueRef
		}
		if n.GetValueBlob().SizeVT() != 0 {
			return ErrUnexpectedBlob
		}
	} else {
		// enforce ValueRef OR ValueBlob
		if n.GetValueRef().SizeVT() != 0 && n.GetValueBlob().SizeVT() != 0 {
			return errors.New("value_blob cannot be set simultaneously with value_ref")
		}
		if err := n.GetValueBlob().Validate(); err != nil {
			return err
		}
	}
	// allow empty left/right refs.
	if err := n.GetLeftChildRef().Validate(true); err != nil {
		return err
	}
	if err := n.GetRightChildRef().Validate(true); err != nil {
		return err
	}
	return nil
}

// IsLeaf checks if the node is a leaf.
func (n *Node) IsLeaf() bool {
	return n.GetHeight() == 0 || n.GetSize() == 0
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (n *Node) MarshalBlock() ([]byte, error) {
	return n.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (n *Node) UnmarshalBlock(data []byte) error {
	return n.UnmarshalVT(data)
}

// ApplySubBlock applies a sub-block change with a field id.
func (n *Node) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 8:
		v, ok := next.(*blob.Blob)
		if !ok {
			return block.ErrUnexpectedType
		}
		n.ValueBlob = v
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (n *Node) GetSubBlocks() map[uint32]block.SubBlock {
	if n.GetValueBlob() == nil {
		return nil
	}
	m := make(map[uint32]block.SubBlock)
	m[8] = n.GetValueBlob()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (n *Node) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 8:
		return blob.NewBlobSubBlockCtor(&n.ValueBlob)
	}
	return nil
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (n *Node) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 5:
		n.LeftChildRef = ptr
	case 6:
		n.RightChildRef = ptr
	case 7:
		n.ValueRef = ptr
	}
	return nil
}

// FollowLeft follows the left child.
func (n *Node) FollowLeft(ctx context.Context, cursor *block.Cursor) (*Node, *block.Cursor, error) {
	bcs := cursor.FollowRef(5, n.GetLeftChildRef())
	bcv, err := loadNode(ctx, bcs)
	return bcv, bcs, err
}

// FollowRight follows the right child.
func (n *Node) FollowRight(ctx context.Context, cursor *block.Cursor) (*Node, *block.Cursor, error) {
	bcs := cursor.FollowRef(6, n.GetRightChildRef())
	bcv, err := loadNode(ctx, bcs)
	return bcv, bcs, err
}

// FollowValue follows the value reference or blob.
// Returns the cursor to the value and whether it is a blob.
func (n *Node) FollowValue(cursor *block.Cursor) (*block.Cursor, bool) {
	if n.ValueIsBlob() {
		return cursor.FollowSubBlock(8), true
	}
	return cursor.FollowRef(7, n.GetValueRef()), false
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (n *Node) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{
		5: n.GetLeftChildRef(),
		6: n.GetRightChildRef(),
		7: n.GetValueRef(),
	}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID.
func (n *Node) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 5:
		return NewNodeBlock
	case 6:
		return NewNodeBlock

		// Unknown!
		// case 7:
		// return byteslice.NewByteSliceBlock
	}
	return nil
}

// GetBlockGraphAttributes returns the block graph attributes.
func (n *Node) GetBlockGraphAttributes() []encoding.Attribute {
	return []encoding.Attribute{{
		Key: "label",
		Value: fmt.Sprintf(
			"key: %q\nsize: %d\nheight: %d",
			n.GetKey(),
			n.GetSize(),
			n.GetHeight(),
		),
	}}
}

// _ is a type assertion
var (
	_ block.Block               = ((*Node)(nil))
	_ block.BlockWithAttributes = ((*Node)(nil))
	_ block.BlockWithRefs       = ((*Node)(nil))
	_ block.BlockWithSubBlocks  = ((*Node)(nil))
)
