//go:build darwin || linux || windows

package s4db

import "context"

// openInProcessError is returned when this process already has the file open.
// Wait blocks until that claim is released.
type openInProcessError struct {
	// id is the file the process already has open.
	id fileID
}

// Error returns the in-process open sentinel text.
func (e *openInProcessError) Error() string {
	return ErrOpenInProcess.Error()
}

// Unwrap returns the in-process open sentinel.
func (e *openInProcessError) Unwrap() error {
	return ErrOpenInProcess
}

// Is reports that target is the in-process open sentinel.
func (e *openInProcessError) Is(target error) bool {
	return target == ErrOpenInProcess
}

// Wait blocks until the process releases the file, or ctx ends.
func (e *openInProcessError) Wait(ctx context.Context) error {
	return openFiles.waitRelease(ctx, e.id)
}
