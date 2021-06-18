package execution_transaction

import (
	"github.com/aperturerobotics/hydra/world"
	"github.com/golang/protobuf/proto"
)

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *ExecutionTxData) MarshalBlock() ([]byte, error) {
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *ExecutionTxData) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// _ is a type assertion
var _ world.Operation = ((*ExecutionTxData)(nil))
