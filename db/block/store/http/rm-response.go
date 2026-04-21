package block_store_http

import (
	"github.com/s4wave/spacewave/db/block"
)

// MarshalBlock marshals the block to binary.
func (o *RmResponse) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (o *RmResponse) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*RmResponse)(nil))
