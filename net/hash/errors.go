package hash

import "github.com/pkg/errors"

// ErrHashTypeUnsupported is returned when persisted state names a hash type this build cannot use.
var ErrHashTypeUnsupported = errors.New("unsupported hash type")

// UnsupportedHashTypeError records an unsupported hash type while preserving the original error text.
type UnsupportedHashTypeError struct {
	HashType HashType
	Message  string
}

// Error returns the unsupported hash error message.
func (e *UnsupportedHashTypeError) Error() string {
	if e == nil {
		return ErrHashTypeUnsupported.Error()
	}
	return e.Message
}

// Unwrap exposes ErrHashTypeUnsupported for errors.Is.
func (e *UnsupportedHashTypeError) Unwrap() error {
	return ErrHashTypeUnsupported
}

// newUnsupportedHashTypeError returns an UnsupportedHashTypeError for a hash
// type this build cannot use.
func newUnsupportedHashTypeError(hashType HashType, message string) error {
	return &UnsupportedHashTypeError{HashType: hashType, Message: message}
}
