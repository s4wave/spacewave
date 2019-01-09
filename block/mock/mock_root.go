package block_mock

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/golang/protobuf/proto"
)

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *Root) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *Root) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// ApplyRef applies a ref change with a field id.
func (r *Root) ApplyRef(id uint32, ptr *cid.BlockRef) error {
	switch id {
	case 1:
		r.ExamplePtr = ptr
	}
	return nil
}

// _ is a type assertion
var _ block.Block = ((*Root)(nil))
