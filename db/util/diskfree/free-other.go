//go:build !unix && !windows

package diskfree

import "github.com/pkg/errors"

// ErrUnknown is returned on a platform with no free space query, so a caller
// refuses rather than fill a disk it cannot measure.
var ErrUnknown = errors.New("free space unknown on this platform")

// Bytes returns ErrUnknown.
func Bytes(string) (uint64, error) {
	return 0, ErrUnknown
}
