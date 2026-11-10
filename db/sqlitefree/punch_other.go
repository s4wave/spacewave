//go:build !linux

package sqlitefree

import (
	"os"

	"github.com/pkg/errors"
)

// Supported reports whether PunchRanges can release blocks on this system.
const Supported = false

// PunchRanges needs fallocate(PUNCH_HOLE), which only Linux has here.
func PunchRanges(*os.File, int64, []Range) error {
	return errors.New("punch needs Linux")
}

// IsHole reports no hole off Linux.
func IsHole(*os.File, int64, Range) bool { return false }
