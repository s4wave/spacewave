package unixfs_world

import (
	"errors"

	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/world"
)

// fsRefusals are the unixfs errors an FS operation returns when its paths do
// not fit the tree it applies to. Only the unixfs tree code returns them, from
// the tree's decoded nodes, so each is a decision of the operation and its
// base, never a failure to read the base.
//
// ErrExist and ErrNotExist are missing on purpose: they are fs.ErrExist and
// fs.ErrNotExist, which a disk-backed block store can also return when it
// fails to read, so they wait until the tree returns its own sentinel.
var fsRefusals = []error{
	unixfs_errors.ErrNotDirectory,
	unixfs_errors.ErrNotFile,
	unixfs_errors.ErrNotSymlink,
	unixfs_errors.ErrEmptyPath,
	unixfs_errors.ErrAbsolutePath,
	unixfs_errors.ErrMoveToSelf,
}

// fsRefusal returns err as a *world.OpRefusedError when it is one of
// fsRefusals, and err unchanged otherwise.
func fsRefusal(err error) error {
	for _, refusal := range fsRefusals {
		if errors.Is(err, refusal) {
			return world.NewOpRefusedError(err)
		}
	}
	return err
}
