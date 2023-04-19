package block_store_http

import (
	"github.com/aperturerobotics/hydra/block"
)

// Validate validates the put request.
func (o *PutRequest) Validate() error {
	if o == nil {
		return nil
	}
	if len(o.GetData()) == 0 {
		return block.ErrEmptyBlock
	}
	if err := o.GetPutOpts().Validate(); err != nil {
		return err
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *PutRequest) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *PutRequest) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*PutRequest)(nil))
