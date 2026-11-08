package msgpack

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/vmihailenco/msgpack/v5"
)

// MsgpackBlock directly wraps an interface with a decoder/encoder.
type MsgpackBlock[T any] struct {
	alias block.AliasIdentityToken
	obj   T
}

// NewMsgpackBlock builds a new object wrapped with a msgpack decoder.
//
// Obj should be a pointer to the field to decode / encode.
func NewMsgpackBlock[T any](obj T) *MsgpackBlock[T] {
	return &MsgpackBlock[T]{obj: obj}
}

// UnmarshalMsgpackBlock loads a msgpack block at a cursor.
//
// If ctor is nil, uses the empty value of T. May return nil.
func UnmarshalMsgpackBlock[T any](ctx context.Context, bcs *block.Cursor, ctor func() T) (*MsgpackBlock[T], error) {
	// Return a nil block for an absent cursor.
	if bcs == nil {
		return nil, nil
	}

	// Decode into a block wrapping the constructed value.
	blk, err := bcs.Unmarshal(ctx, func() block.Block {
		if ctor == nil {
			var empty T
			return NewMsgpackBlock(empty)
		}
		return NewMsgpackBlock(ctor())
	})
	if err != nil || blk == nil {
		return nil, err
	}

	// Require the cursor block to be a msgpack block of T.
	out, ok := blk.(*MsgpackBlock[T])
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return out, nil
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
//
// T and dest can be a nil interface{} to unmarshal a dynamic type. If bcs is
// nil, returns dest, nil.
func BlockToObject[T comparable](ctx context.Context, bcs *block.Cursor, dest T) (T, error) {
	// Preserve the destination when no block cursor was supplied.
	if bcs == nil {
		return dest, nil
	}

	// Load the MessagePack block using the supplied destination.
	b, err := UnmarshalMsgpackBlock(ctx, bcs, func() T {
		return dest
	})
	if err != nil {
		return dest, err
	}

	// Decode cached block bytes again when they refer to another destination.
	out := b.obj
	if out != dest {
		// Fetch the MessagePack bytes for the replacement destination.
		data, found, err := bcs.FetchLeaf(ctx)
		if err != nil {
			return dest, err
		}
		if !found {
			return dest, block.ErrNotFound
		}

		// Decode the fetched bytes into the supplied destination.
		b = &MsgpackBlock[T]{obj: dest}
		err = b.UnmarshalVT(data)
		if err != nil {
			return dest, err
		}
		out = dest
	}

	return out, nil
}

// IsNil checks if the object is nil.
func (b *MsgpackBlock[T]) IsNil() bool {
	return b == nil
}

// BlockAliasIdentity returns the in-memory alias token for MsgpackBlock.
func (b *MsgpackBlock[T]) BlockAliasIdentity() *block.AliasIdentityToken {
	return &b.alias
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

// MarshalVT marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *MsgpackBlock[T]) MarshalVT() ([]byte, error) {
	return msgpack.Marshal(b.obj)
}

// UnmarshalVT unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *MsgpackBlock[T]) UnmarshalVT(data []byte) error {
	return msgpack.Unmarshal(data, &b.obj)
}

// _ is a type assertion
var (
	_ block.Block    = (*MsgpackBlock[any])(nil)
	_ block.SubBlock = (*MsgpackBlock[any])(nil)
)
