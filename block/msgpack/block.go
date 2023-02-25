package msgpack

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/vmihailenco/msgpack/v5"
)

// MsgpackBlock directly wraps an interface with a decoder/encoder.
type MsgpackBlock[T any] struct {
	obj T
}

// NewMsgpackBlock builds a new object wrapped with a msgpack decoder.
//
// Obj should be a pointer to the field to decode / encode.
func NewMsgpackBlock[T any](obj T) *MsgpackBlock[T] {
	return &MsgpackBlock[T]{obj: obj}
}

// UnmarshalMsgpackBlock loads a msgpack block at a cursor.
// if ctor is nil, uses the empty value of T.
// may return nil
func UnmarshalMsgpackBlock[T any](bcs *block.Cursor, ctor func() T) (*MsgpackBlock[T], error) {
	return block.UnmarshalBlock[*MsgpackBlock[T]](bcs, func() block.Block {
		if ctor == nil {
			var empty T
			return NewMsgpackBlock(empty)
		}
		if ctor == nil {
			var empty T
			return NewMsgpackBlock(empty)
		} else {
			return NewMsgpackBlock(ctor())
		}
	})
}

// ObjectToBlock converts a given object into a msgpack block at bcs.
func ObjectToBlock[T any](bcs *block.Cursor, obj T) error {
	if bcs == nil {
		return block.ErrNilCursor
	}
	bcs.ClearAllRefs()
	bcs.SetBlock(&MsgpackBlock[T]{obj: obj}, true)
	return nil
}

// BlockToObject converts the given block cursor into an object.
// T and dest can be a nil interface{} to unmarshal a dynamic type.
// if bcs is nil returns dest, nil
func BlockToObject[T comparable](bcs *block.Cursor, dest T) (T, error) {
	if bcs == nil {
		return dest, nil
	}
	b, err := UnmarshalMsgpackBlock(bcs, func() T {
		return dest
	})
	if err != nil {
		return dest, err
	}
	out := b.obj
	if out != dest {
		// different object, re-parse
		data, found, err := bcs.Fetch()
		if err != nil {
			return dest, err
		}
		if !found {
			return dest, block.ErrNotFound
		}
		b = &MsgpackBlock[T]{obj: dest}
		err = b.UnmarshalBlock(data)
		if err != nil {
			return dest, block.ErrNotFound
		}
		out = dest
	}
	return out, nil
}

// GetObj returns the contained object.
func (b *MsgpackBlock[T]) GetObj() T {
	if b == nil {
		var empty T
		return empty
	}
	return b.obj
}

// SetObj sets the contained object.
func (b *MsgpackBlock[T]) SetObj(obj T) {
	b.obj = obj
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *MsgpackBlock[T]) MarshalBlock() ([]byte, error) {
	return msgpack.Marshal(b.obj)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *MsgpackBlock[T]) UnmarshalBlock(data []byte) error {
	return msgpack.Unmarshal(data, &b.obj)
}

// _ is a type assertion
var (
	_ block.Block    = ((*MsgpackBlock[interface{}])(nil))
	_ block.SubBlock = ((*MsgpackBlock[interface{}])(nil))
)
