package store

import "github.com/pkg/errors"

// ErrPackfileStoreClosed is returned when a read opens a pack after shutdown.
var ErrPackfileStoreClosed = errors.New("packfile store: closed")

// ErrIncompleteCachedPackRange is returned when cached shared spans do not
// fully cover the bytes for a block that the range metadata claims exists.
var ErrIncompleteCachedPackRange = errors.New("packfile store: incomplete cached pack range")
