package unixfs_access

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
)

// AccessUnixFSFunc is a function to access a UnixFS.
// Optionally pass a released function that may be called when the handle was released.
// Returns a release function.
type AccessUnixFSFunc = func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error)

// NewAccessUnixFSFunc constructs a AccessUnixFSFunc from a FSHandle.
func NewAccessUnixFSFunc(handle *unixfs.FSHandle) AccessUnixFSFunc {
	return func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
		handle, err := handle.Clone(ctx)
		if err != nil {
			return nil, nil, err
		}
		handle.AddReleaseCallback(released)
		return handle, handle.Release, nil
	}
}

// NewAccessUnixFSViaBusFunc builds a new func which accesses the UnixFS on the
// given bus using the AccessUnixFS directive.
//
// If returnIfIdle is set: ErrFsNotFound is returned if not found.
func NewAccessUnixFSViaBusFunc(b bus.Bus, unixfsID string, returnIfIdle bool) AccessUnixFSFunc {
	return func(ctx context.Context, released func()) (*unixfs.FSHandle, func(), error) {
		// access the directive via the bus
		val, ref, err := ExAccessUnixFS(ctx, b, unixfsID, returnIfIdle, released)
		if err == nil && val == nil {
			err = unixfs_errors.ErrFsNotFound
		}
		if err != nil {
			if ref != nil {
				ref.Release()
			}
			return nil, nil, err
		}

		// call the inner access function
		result, relResult, err := val(ctx, released)
		rel := ref.Release
		if relResult != nil {
			rel = func() {
				relResult()
				ref.Release()
			}
		}
		if err != nil {
			rel()
			return nil, nil, err
		}

		return result, rel, nil
	}
}
