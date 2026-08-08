package coord

import "errors"

// ErrUnsupported indicates a scoped ObjectStore does not support direct coordination.
var ErrUnsupported = errors.New("coord: unsupported")
