//go:build !unix && !windows

package extsort

import "github.com/pkg/errors"

// freeBytes reports that this platform has no free space query, so Reserve
// refuses rather than spill into a disk it cannot measure.
func freeBytes(string) (int64, error) {
	return 0, errors.New("free space unknown on this platform")
}
