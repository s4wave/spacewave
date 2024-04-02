package unixfs_world

import "errors"

// ErrInvalidFSType is returned if the FSType was not recognized.
var ErrInvalidFSType = errors.New("invalid fs type")
