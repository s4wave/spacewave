package s4db

import "github.com/pkg/errors"

var (
	// ErrUnimplemented is returned by every operation of the stub.
	ErrUnimplemented = errors.New("s4db is being rewritten")
	// ErrOpenInProcess is returned when this process already has the file
	// open.
	ErrOpenInProcess = errors.New("database is already open in this process")
	// ErrClosed is returned when the database is closed.
	ErrClosed = errors.New("database is closed")
)
