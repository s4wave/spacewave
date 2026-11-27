package world

import (
	"errors"

	"github.com/s4wave/spacewave/db/block"
)

// OpRefusedError is an operation its handler refused on the World it applied
// to: a decision of the handler from the operation and that base alone, such
// as a target that references another object or an object of the wrong type.
// Every replica that applies the operation to the same base refuses it the
// same way. A failure to read the base, such as a lookup or storage error, is
// never one: it depends on the device and must wait.
type OpRefusedError struct {
	// Err is the handler's refusal.
	Err error
}

// Error returns the handler's refusal message.
func (e *OpRefusedError) Error() string { return e.Err.Error() }

// Unwrap returns the handler's refusal, so errors.Is still matches the
// sentinel it wraps.
func (e *OpRefusedError) Unwrap() error { return e.Err }

// NewOpRefusedError wraps err, a handler's refusal, in an *OpRefusedError.
// It returns nil for nil, and err unchanged when it already is one.
func NewOpRefusedError(err error) error {
	if err == nil || IsOpRefusedError(err) {
		return err
	}
	return &OpRefusedError{Err: err}
}

// IsOpRefusedError reports whether err is or wraps an *OpRefusedError. A
// handler whose write callback can both refuse and fail to read returns
// !IsOpRefusedError(err) as its sysErr, so only the read failure waits.
func IsOpRefusedError(err error) bool {
	var refused *OpRefusedError
	return errors.As(err, &refused)
}

// RefuseDecodeError refuses err when it is a *block.DecodeError: bytes that
// were read and checked against their ref but do not decode, the same on
// every replica. Any other error, such as a failed read, is returned
// unchanged and waits.
func RefuseDecodeError(err error) error {
	if _, ok := errors.AsType[*block.DecodeError](err); ok {
		return NewOpRefusedError(err)
	}
	return err
}
