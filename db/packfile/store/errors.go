package store

import "github.com/pkg/errors"

// ErrPackfileStoreClosed is returned when a read opens a pack after shutdown.
var ErrPackfileStoreClosed = errors.New("packfile store: closed")

// ErrPackReaderClosed is returned by a read on a pack reader that was closed,
// because the store shut down or a manifest change removed its pack.
var ErrPackReaderClosed = errors.New("packfile store: pack reader closed")
