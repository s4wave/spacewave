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
// ErrExist and ErrNotExist are unixfs values distinct from fs.ErrExist and
// fs.ErrNotExist, so an OS error a disk-backed store returns never matches
// them.
var fsRefusals = []error{
	unixfs_errors.ErrExist,
	unixfs_errors.ErrNotExist,
	unixfs_errors.ErrNotDirectory,
	unixfs_errors.ErrNotFile,
	unixfs_errors.ErrNotSymlink,
	unixfs_errors.ErrEmptyPath,
	unixfs_errors.ErrAbsolutePath,
	unixfs_errors.ErrMoveToSelf,
	unixfs_errors.ErrNegativeOffset,
	unixfs_errors.ErrEmptyBlob,
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
