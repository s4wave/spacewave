package block_store_http

import (
	"github.com/aperturerobotics/hydra/block"
)

// MarshalBlock marshals the block to binary.
func (o *ExistsResponse) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *ExistsResponse) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*ExistsResponse)(nil))
