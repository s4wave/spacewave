package iavl

import (
	"fmt"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"gonum.org/v1/gonum/graph/encoding"
)

// NewNodeBlock constructs a new node block.
func NewNodeBlock() block.Block {
	return &Node{}
}

// loadNode follows the node cursor.
// may return nil
func loadNode(cursor *block.Cursor) (*Node, error) {
	ni, err := cursor.Unmarshal(func() block.Block { return &Node{} })
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

// Validate does cursory checks on the node.
func (n *Node) Validate() error {
	if n.GetHeight() != 0 && len(n.GetValue()) != 0 {
		return errors.New("unexpected value in non-leaf node")
	}
	if err := n.GetLeftChildRef().Validate(); err != nil {
		return err
	}
	if err := n.GetRightChildRef().Validate(); err != nil {
		return err
	}
	return nil
}

// IsLeaf checks if the node is a leaf.
func (n *Node) IsLeaf() bool {
	return n.GetHeight() == 0
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (n *Node) MarshalBlock() ([]byte, error) {
	return proto.Marshal(n)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (n *Node) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, n)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (n *Node) ApplyBlockRef(id uint32, ptr *block.BlockRef) error {
	switch id {
	case 5:
		n.LeftChildRef = ptr
	case 6:
		n.RightChildRef = ptr
	}
	return nil
}

// FollowLeft follows the left child.
func (n *Node) FollowLeft(cursor *block.Cursor) (*Node, *block.Cursor, error) {
	bcs := cursor.FollowRef(5, n.GetLeftChildRef())
	bcv, err := loadNode(bcs)
	return bcv, bcs, err
}

// FollowRight follows the right child.
func (n *Node) FollowRight(cursor *block.Cursor) (*Node, *block.Cursor, error) {
	bcs := cursor.FollowRef(6, n.GetRightChildRef())
	bcv, err := loadNode(bcs)
	return bcv, bcs, err
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (n *Node) GetBlockRefs() (map[uint32]*block.BlockRef, error) {
	return map[uint32]*block.BlockRef{
		5: n.GetLeftChildRef(),
		6: n.GetRightChildRef(),
	}, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID.
func (n *Node) GetBlockRefCtor(id uint32) block.Ctor {
	switch id {
	case 5:
		fallthrough
	case 6:
		return NewNodeBlock
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

// ApplySubBlock applies a sub-block change with a field id.
func (n *Node) ApplySubBlock(id uint32, next block.SubBlock) error {
	switch id {
	case 4:
		b, bOk := next.(block.Block)
		if !bOk {
			return ErrMustBeBlock
		}
		d, err := b.MarshalBlock()
		if err != nil {
			return err
		}
		n.Value = d
		return nil
	default:
		return errors.Errorf("unexpected sub-block id: %d", id)
	}
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (n *Node) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[4] = byteslice.NewByteSlice(&n.Value)
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (n *Node) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 4:
		return func(create bool) block.SubBlock {
			return byteslice.NewByteSlice(&n.Value)
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block               = ((*Node)(nil))
	_ block.BlockWithAttributes = ((*Node)(nil))
	_ block.BlockWithSubBlocks  = ((*Node)(nil))
)
