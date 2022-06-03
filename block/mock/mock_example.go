package block_mock

import (
	"github.com/aperturerobotics/hydra/block"
	"google.golang.org/protobuf/proto"
)

// NewExample builds a new example block with a message.
func NewExample(msg string) *Example {
	return &Example{Msg: msg}
}

// NewExampleBlock builds a new example block.
func NewExampleBlock() block.Block {
	return &Example{}
}

// UnmarshalExample unmarshals the example block.
// Returns nil, nil if empty
func UnmarshalExample(bcs *block.Cursor) (*Example, error) {
	exi, err := bcs.Unmarshal(NewExampleBlock)
	if err != nil {
		return nil, err
	}
	if exi == nil {
		return nil, nil
	}
	ex, ok := exi.(*Example)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return ex, nil
}

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

// _ is a type assertion
var _ block.Block = ((*Example)(nil))
