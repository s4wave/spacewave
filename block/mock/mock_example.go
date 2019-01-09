package block_mock

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/golang/protobuf/proto"
)

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Example) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Example) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// ApplyRef applies a ref change with a field id.
func (e *Example) ApplyRef(id uint32, ptr *cid.BlockRef) error {
	return nil
}

// _ is a type assertion
var _ block.Block = ((*Example)(nil))
