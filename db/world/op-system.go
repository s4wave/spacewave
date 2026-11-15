package world

import "errors"

// ErrOpSystemFault is the cause of an OpSystemError whose handler reported a
// system error with no error of its own.
var ErrOpSystemFault = errors.New("operation handler reported a system error")

// OpSystemError is the failure of an operation whose handler reported it as a
// system error: a fault of the device or the system applying the operation,
// not a verdict on the operation, so a later attempt may apply it. It is never
// a refusal, even when Err is an error a refusal also returns.
type OpSystemError struct {
	// Err is the handler's error, or ErrOpSystemFault.
	Err error
}

// NewOpSystemError returns err as an OpSystemError, or one with
// ErrOpSystemFault when err is nil.
func NewOpSystemError(err error) error {
	if err == nil {
		err = ErrOpSystemFault
	}
	var sysErr *OpSystemError
	if errors.As(err, &sysErr) {
		return err
	}
	return &OpSystemError{Err: err}
}

// Error returns the error message.
func (e *OpSystemError) Error() string {
	return "system error: " + e.Err.Error()
}

// Unwrap returns the handler's error.
func (e *OpSystemError) Unwrap() error {
	return e.Err
}
