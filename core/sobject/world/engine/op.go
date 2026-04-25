package sobject_world_engine

import (
	"github.com/s4wave/spacewave/db/block"
)

// NewSOWorldOpBlock constructs a new SOWorldOp block.
func NewSOWorldOpBlock() block.Block {
	return &SOWorldOp{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (s *SOWorldOp) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (s *SOWorldOp) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Validate validates the InitWorldOp configuration.
func (i *InitWorldOp) Validate() error {
	if err := i.GetTransformConf().Validate(); err != nil {
		return err
	}
	return nil
}

// _ is a type assertion
var _ block.Block = ((*SOWorldOp)(nil))
