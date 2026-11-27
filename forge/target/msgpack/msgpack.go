// Package forge_target_msgpack stores and loads Forge values as MessagePack
// blocks. It is apart from forge/target so that a program which handles
// targets without MessagePack values links no MessagePack code.
package forge_target_msgpack

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_msgpack "github.com/s4wave/spacewave/db/block/msgpack"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// StoreValue stores the given data as one MessagePack block.
func StoreValue(
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	value any,
) (*forge_value.Value, error) {
	return forge_target.AccessValue(ctx, handle, nil, func(bcs *block.Cursor) error {
		err := block_msgpack.ObjectToBlock(bcs, value)
		return err
	})
}

// LoadValue loads the data from a MessagePack block. Use T any to unmarshal
// dynamic types. An empty value loads as ctor's result, or the zero T when
// ctor is nil.
func LoadValue[T any](
	ctx context.Context,
	handle forge_target.ExecControllerHandle,
	value *forge_value.Value,
	ctor func() T,
) (T, error) {
	// Return the requested default value when the msgpack value is empty.
	if value.IsEmpty() {
		if ctor == nil {
			var empty T
			return empty, nil
		}
		return ctor(), nil
	}

	// Decode the stored msgpack value through its block cursor.
	var outObj T
	_, err := forge_target.AccessValue(ctx, handle, value, func(bcs *block.Cursor) error {
		outBlk, berr := block_msgpack.UnmarshalMsgpackBlock(ctx, bcs, ctor)
		if berr != nil {
			return berr
		}
		outObj = outBlk.GetObj()
		return nil
	})
	if err != nil {
		var empty T
		return empty, err
	}
	return outObj, nil
}
