package unixfs_world

import "errors"

// ErrInvalidFSType is returned if the FSType was not recognized.
var ErrInvalidFSType = errors.New("invalid fs type")

// ErrFSObjectUnsupported is returned for an FS_OBJECT filesystem, which no
// reader or writer implements yet.
var ErrFSObjectUnsupported = errors.New("fs object filesystems are not supported")
