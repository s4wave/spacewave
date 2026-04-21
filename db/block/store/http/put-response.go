package block_store_http

import (
	"github.com/s4wave/spacewave/db/block"
)

// Validate validates the put response.
func (o *PutResponse) Validate() error {
	if o == nil {
		return nil
	}
	// allow empty ref only if err is not empty
	return o.GetRef().Validate(o.GetErr() != "")
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (o *PutResponse) MarshalBlock() ([]byte, error) {
	return o.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (o *PutResponse) UnmarshalBlock(data []byte) error {
	return o.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*PutResponse)(nil))
