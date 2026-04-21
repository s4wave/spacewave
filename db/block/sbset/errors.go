package sbset

import "errors"

var (
	// ErrNonUniqueName is returned if a non-unique name is found.
	ErrNonUniqueName = errors.New("sub-block name is not unique")
	// ErrEmptyName is returned if a empty name is found.
	ErrEmptyName = errors.New("sub-block name is empty")
)
