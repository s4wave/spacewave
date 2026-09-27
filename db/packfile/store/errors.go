package store

import "github.com/pkg/errors"

// ErrPackfileStoreClosed is returned when a read opens a pack after shutdown.
var ErrPackfileStoreClosed = errors.New("packfile store: closed")
