//go:build !sql_lite

package block_store

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
)

// AccessBlockStoreFunc is a function to access a BlockStoreHandle.
// Optionally pass a released function that may be called when the handle was released.
// Returns a release function.
type AccessBlockStoreFunc = func(ctx context.Context, released func()) (Store, func(), error)

// NewAccessBlockStoreViaBusFunc builds a new func which accesses the BlockStore on the
// given bus using the LookupBlocKStore directive.
//
// If returnIfIdle is set: ErrBlockStoreNotFound is returned if not found.
func NewAccessBlockStoreViaBusFunc(b bus.Bus, blockStoreID string, returnIfIdle bool) AccessBlockStoreFunc {
	return func(ctx context.Context, released func()) (Store, func(), error) {
		// access the directive via the bus
		val, _, ref, err := ExLookupFirstBlockStore(ctx, b, blockStoreID, returnIfIdle, released)
		if err != nil || val == nil {
			if ref != nil {
				ref.Release()
			}
			if err == nil {
				err = ErrBlockStoreNotFound
			}
			return nil, nil, err
		}

		return val, ref.Release, nil
	}
}
