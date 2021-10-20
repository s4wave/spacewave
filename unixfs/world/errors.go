package unixfs_world

import "errors"

var (
	// ErrInvalidFSType is returned if the FSType was not recognized.
	ErrInvalidFSType = errors.New("invalid fs type")
)
