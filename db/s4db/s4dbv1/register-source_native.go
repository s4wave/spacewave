//go:build darwin || linux || windows

package s4dbv1

import (
	"errors"
	"os"

	"github.com/s4wave/spacewave/db/s4db"
)

// openRegisteredSource shares process-level descriptor ownership with the engine.
func openRegisteredSource(path string) (*os.File, func() error, error) {
	// Treat another local descriptor exactly like another process's source lock.
	f, closeFile, err := s4db.OpenFile(path, openSource)
	if errors.Is(err, s4db.ErrOpenInProcess) {
		return nil, nil, ErrInUse
	}
	return f, closeFile, err
}
