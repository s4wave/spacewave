package unixfs_access

import (
	"context"

	"github.com/aperturerobotics/hydra/unixfs"
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
