//go:build darwin || linux || windows

package s4db

import (
	"context"
	"os"
)

// Wait blocks until the process that owns the writable file closes it, or ctx
// ends, so a Volume controller retries the open as a runtime handoff instead
// of giving up. It takes the owner byte and releases it at once: another
// waiter may claim the file first, and the retried open then waits again.
func (e *WriterOwnedError) Wait(ctx context.Context) error {
	// A refusal without a path references no file to wait on.
	if e.path == "" {
		return e
	}

	// Wait on the owner byte through a descriptor owned by Wait, then close
	// the descriptor.
	f, closeFile, err := OpenFile(e.path, func(name string) (*os.File, error) {
		return openNativeFile(name, os.O_RDWR)
	})
	if err != nil {
		return err
	}
	s := &osFile{File: f, path: e.path, closeFile: closeFile}
	if err := s.waitOwner(ctx); err != nil {
		return err
	}
	return s.close()
}
