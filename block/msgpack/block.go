package msgpack

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/vmihailenco/msgpack/v5"
)

// MsgpackBlock directly wraps an interface with a decoder/encoder.
type MsgpackBlock struct {
	obj interface{}
}

// NewMsgpackBlock builds a new object wrapped with a msgpack decoder.
//
// Obj should be a pointer to the field to decode / encode.
func NewMsgpackBlock(obj interface{}) *MsgpackBlock {
	if obj == nil {
		// construct a spot in memory to hold the type info.
		var nobj interface{}
		obj = &nobj
	}
	return &MsgpackBlock{obj: obj}
}

// GetObj returns the contained object.
func (b *MsgpackBlock) GetObj() interface{} {
	return b.obj
}

// SetObj sets the contained object.
func (b *MsgpackBlock) SetObj(obj interface{}) {
	b.obj = obj
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *MsgpackBlock) MarshalBlock() ([]byte, error) {
	return msgpack.Marshal(b.obj)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *MsgpackBlock) UnmarshalBlock(data []byte) error {
	return msgpack.Unmarshal(data, b.obj)
}

// _ is a type assertion
var (
	_ block.Block    = ((*MsgpackBlock)(nil))
	_ block.SubBlock = ((*MsgpackBlock)(nil))
)
