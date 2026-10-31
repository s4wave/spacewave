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
	// ErrPreviousFormat is returned, joined with ErrUnsupported, when the
	// file was written by the previous engine and can be converted.
	ErrPreviousFormat = errors.New("s4wave database in the previous format")
	// ErrCorrupt is returned when stored bytes fail their checksum.
	ErrCorrupt = errors.New("s4wave database checksum mismatch")
	// ErrSlotsFull is returned when every reader slot is held.
	ErrSlotsFull = errors.New("every reader slot is in use")
	// ErrOpenInProcess is returned when this process already has the file
	// open. Record locks on some systems belong to the process, so a second
	// handle could not exclude the first.
	ErrOpenInProcess = errors.New("database is already open in this process")
	// ErrReadOnly is returned when a read-only handle is asked to write.
	ErrReadOnly = errors.New("database is open read-only")
	// ErrKeyTooLong is returned when a key exceeds MaxKeySize.
	ErrKeyTooLong = errors.New("key exceeds the maximum key size")
	// ErrValueTooLarge is returned when a value exceeds MaxValueSize.
	ErrValueTooLarge = errors.New("value exceeds the maximum value size")
	// ErrClosed is returned when the database is closed.
	ErrClosed = errors.New("database is closed")
	// ErrOutOfOrder is returned by the bulk builder when a key is not above
	// the one before it.
	ErrOutOfOrder = errors.New("keys are not in ascending order")
)
