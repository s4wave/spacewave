//go:build !unix && !windows

package unixfs_world

import "github.com/pkg/errors"

// folderRootID reports that this platform cannot identify a folder's file
// system, so it cannot link folders.
func folderRootID(string) (uint64, error) {
	return 0, errors.New("linked folders are not supported on this platform")
}
