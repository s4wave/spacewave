package blob

import "errors"

var (
	// ErrRawBlobSizeMismatch is returned if the raw blob size field does not match the data len.
	ErrRawBlobSizeMismatch = errors.New("raw blob size must match data len")
)
