package coord

import "errors"

var (
	// ErrUnsupported indicates a scoped ObjectStore does not support direct coordination.
	ErrUnsupported = errors.New("coord: unsupported")
)
