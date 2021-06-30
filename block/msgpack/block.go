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

// NewMsgpackBlockBlock is the block constructor for MsgpackBlock
func NewMsgpackBlockBlock() block.Block {
	return &MsgpackBlock{}
}

// UnmarshalMsgpackBlock loads a msgpack block at a cursor.
// initObject can be nil to indicate unmarshaling dynamic type.
// if bcs already contained a block, initObject will be ignored.
// may return nil
func UnmarshalMsgpackBlock(bcs *block.Cursor, initObject interface{}) (*MsgpackBlock, error) {
	ni, err := bcs.Unmarshal(func() block.Block {
		b := &MsgpackBlock{}
		if initObject != nil {
			b.obj = initObject
		}
		return b
	})
	if err != nil {
		return nil, err
	}
	niv, _ := ni.(*MsgpackBlock)
	return niv, nil
}

// ObjectToBlock converts a given object into a msgpack block at bcs.
func ObjectToBlock(bcs *block.Cursor, obj interface{}) error {
	if bcs == nil {
		return block.ErrNilCursor
	}
	bcs.ClearAllRefs()
	bcs.SetBlock(&MsgpackBlock{obj: obj}, true)
	return nil
}

// BlockToObject converts the given block cursor into an object.
// if dest is nil, uses a dynamic type.
// if bcs is nil returns dest, nil
func BlockToObject(bcs *block.Cursor, dest interface{}) (interface{}, error) {
	if bcs == nil {
		return dest, nil
	}
	b, err := UnmarshalMsgpackBlock(bcs, dest)
	if err != nil {
		return nil, err
	}
	out := b.obj
	if out != dest {
		// different object, re-parse
		data, found, err := bcs.Fetch()
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, block.ErrNotFound
		}
		b = &MsgpackBlock{obj: dest}
		err = b.UnmarshalBlock(data)
		if err != nil {
			return nil, err
		}
		out = dest
	}
	return out, nil
}

// GetObj returns the contained object.
func (b *MsgpackBlock) GetObj() interface{} {
	if b == nil {
		return nil
	}
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
	return msgpack.Unmarshal(data, &b.obj)
}

// _ is a type assertion
var (
	_ block.Block    = ((*MsgpackBlock)(nil))
	_ block.SubBlock = ((*MsgpackBlock)(nil))
)
