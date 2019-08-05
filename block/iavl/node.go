package iavl

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
	cid "github.com/aperturerobotics/hydra/cid"
	"github.com/gogo/protobuf/proto"
)

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

// ApplyRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (n *Node) ApplyRef(id uint32, ptr *cid.BlockRef) error {
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
	bcs, err := cursor.FollowRef(5, n.GetLeftChildRef())
	if err != nil {
		return nil, nil, err
	}
	bcv, err := loadNode(bcs)
	return bcv, bcs, err
}

// FollowRight follows the right child.
func (n *Node) FollowRight(cursor *block.Cursor) (*Node, *block.Cursor, error) {
	bcs, err := cursor.FollowRef(6, n.GetRightChildRef())
	if err != nil {
		return nil, nil, err
	}
	bcv, err := loadNode(bcs)
	return bcv, bcs, err
}

// _ is a type assertion
var _ block.Block = ((*Node)(nil))
