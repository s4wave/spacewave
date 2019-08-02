package btree

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/golang/protobuf/proto"
)

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *Node) MarshalBlock() ([]byte, error) {
	return proto.Marshal(b)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *Node) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, b)
}

// ApplyRef applies a ref change with a field id.
func (b *Node) ApplyRef(id uint32, ptr *cid.BlockRef) error {
	// If refID == 99 then append children ref
	// If refID >= 100 then refs[refID-100] = ptr
	switch {
	case id == 99:
		b.ChildrenRefs = append(b.ChildrenRefs, ptr)
	case id >= 100 && id < 300:
		idx := id - 100
		if len(b.ChildrenRefs) <= int(idx) {
			r := b.ChildrenRefs
			b.ChildrenRefs = make([]*cid.BlockRef, idx+1)
			copy(b.ChildrenRefs, r)
		}
		b.ChildrenRefs[idx] = ptr
	}

	return nil
}

// ChildRefId returns the ref id for a child at index.
func (b *Node) ChildRefId(idx int) uint32 {
	return 100 + uint32(idx)
}

// GetChildrenEmpty returns if there are any children refs.
func (b *Node) GetChildrenEmpty() bool {
	return len(b.GetChildrenRefs()) == 0
}

// _ is a type assertion
var _ block.Block = ((*Node)(nil))
