package volume

import "errors"

// PermanentError marks a volume construction failure that must not be retried.
//
// The volume cannot be constructed in the current environment, so retrying the
// constructor cannot succeed. The browser OPFS backend uses this to report a
// storage-capability denial (a SecurityError on root acquisition) that survives
// a site-data clear and only differs by browser profile. The volume controller
// stops restarting on a permanent error and surfaces it to waiters instead of
// looping forever.
type PermanentError struct {
	// Err is the underlying construction failure.
	Err error
}

// Permanent wraps err as a non-retryable volume construction failure.
// It returns nil when err is nil.
func Permanent(err error) error {
	if err == nil {
		return nil
	}
	return &PermanentError{Err: err}
}

// IsPermanent reports whether err marks a non-retryable construction failure.
func IsPermanent(err error) bool {
	var pe *PermanentError
	return errors.As(err, &pe)
}

// Error returns the underlying error message.
func (e *PermanentError) Error() string {
	return e.Err.Error()
}

// Unwrap returns the underlying error.
func (e *PermanentError) Unwrap() error {
	return e.Err
}
