//go:build !tinygo

package forge_target

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	block_msgpack "github.com/s4wave/spacewave/db/block/msgpack"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// StoreMsgpackValue stores the given data as a Msgpack block.
// The data is all stored in a single block.
func StoreMsgpackValue(
	ctx context.Context,
	handle ExecControllerHandle,
	value any,
) (*forge_value.Value, error) {
	return AccessValue(ctx, handle, nil, func(bcs *block.Cursor) error {
		err := block_msgpack.ObjectToBlock(bcs, value)
		return err
	})
}

// LoadMsgpackValue loads the data from a msgpack block.
// use interface{} type to unmarshal dynamic types.
// if ctor is nil, uses the empty value of T.
// returns the empty value returned from ctor if value is empty
// StoreMsgpackValue stores the given data as a Msgpack block.
func LoadMsgpackValue[T any](
	ctx context.Context,
	handle ExecControllerHandle,
	value *forge_value.Value,
	ctor func() T,
) (T, error) {
	if value.IsEmpty() {
		if ctor == nil {
			var empty T
			return empty, nil
		}
		return ctor(), nil
	}
	var outObj T
	_, err := AccessValue(ctx, handle, value, func(bcs *block.Cursor) error {
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
