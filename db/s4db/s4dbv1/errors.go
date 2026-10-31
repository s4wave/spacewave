package s4dbv1

import (
	"github.com/pkg/errors"
)

var (
	// ErrNotDatabase is returned when a file is not an s4wave database.
	ErrNotDatabase = errors.New("not an s4wave database")
	// ErrUnsupported is returned when the file is not in the previous
	// engine's format.
	ErrUnsupported = errors.New("not a previous-format s4wave database")
	// ErrCorrupt is returned when stored bytes fail their checksum.
	ErrCorrupt = errors.New("s4wave database checksum mismatch")
	// ErrInUse is returned when another process has the file open.
	ErrInUse = errors.New("another process has the database open")
)
