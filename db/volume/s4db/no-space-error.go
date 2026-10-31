//go:build !js && !wasip1

package volume_s4db

import (
	"errors"
	"fmt"

	"github.com/dustin/go-humanize"
)

// ErrNoSpace is returned, through a NoSpaceError, when the disk holding a
// previous-format Volume file cannot hold the converted file beside it.
var ErrNoSpace = errors.New("not enough free disk to convert the volume")

// NoSpaceError says how much disk a conversion needs and how much is free.
type NoSpaceError struct {
	// Need is the bytes the conversion needs free.
	Need uint64
	// Free is the bytes free on the disk.
	Free uint64
}

// Error describes the shortfall.
func (e *NoSpaceError) Error() string {
	return fmt.Sprintf("%s: needs %s free, %s free", ErrNoSpace, humanize.IBytes(e.Need), humanize.IBytes(e.Free))
}

// Unwrap returns ErrNoSpace.
func (e *NoSpaceError) Unwrap() error {
	return ErrNoSpace
}
