package protocol

import "errors"

var (
	// ErrEmptyProtocolID is returned if the protocol id was empty.
	ErrEmptyProtocolID = errors.New("protocol id cannot be empty")
	// ErrInvalidProtocolID is returned if the protocol id was invalid.
	ErrInvalidProtocolID = errors.New("protocol id was invalid")
)
