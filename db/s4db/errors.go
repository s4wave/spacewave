package s4db

import (
	"github.com/pkg/errors"
)

var (
	// ErrNotDatabase is returned when a file is not an s4wave database.
	ErrNotDatabase = errors.New("not an s4wave database")
	// ErrUnsupported is returned when a file uses a page size, feature, or
	// algorithm this version does not implement.
	ErrUnsupported = errors.New("unsupported s4wave database format")
	// ErrCorrupt is returned when stored bytes fail their checksum.
	ErrCorrupt = errors.New("s4wave database checksum mismatch")
	// ErrSlotsFull is returned when every reader slot is held.
	ErrSlotsFull = errors.New("every reader slot is in use")
	// ErrOpenInProcess is returned when this process already has the file
	// open. Record locks on some systems belong to the process, so a second
	// handle could not exclude the first.
	ErrOpenInProcess = errors.New("database is already open in this process")
	// ErrKeyTooLong is returned when a key exceeds MaxKeySize.
	ErrKeyTooLong = errors.New("key exceeds the maximum key size")
	// ErrValueTooLarge is returned when a value exceeds MaxValueSize.
	ErrValueTooLarge = errors.New("value exceeds the maximum value size")
	// ErrTxTooLarge is returned when a commit record would exceed 4 GiB.
	ErrTxTooLarge = errors.New("transaction exceeds the maximum commit size")
	// ErrClosed is returned when the database is closed.
	ErrClosed = errors.New("database is closed")
)
