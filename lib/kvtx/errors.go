package forge_kvtx

import "errors"

var (
	// ErrEmptyKey is returned if the key was not specified in the op
	ErrEmptyKey = errors.New("key was not specified for operation")
	// ErrUnknownOpType is returned if the operation type was unknown.
	ErrUnknownOpType = errors.New("operation type was unknown")
	// ErrValueMismatch is returned if the values did not match.
	ErrValueMismatch = errors.New("values did not match")
)
