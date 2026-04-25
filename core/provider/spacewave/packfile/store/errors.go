package store

import "github.com/pkg/errors"

// ErrIncompleteCachedPackRange is returned when cached shared spans do not
// fully cover the bytes for a block that the range metadata claims exists.
var ErrIncompleteCachedPackRange = errors.New("packfile store: incomplete cached pack range")
