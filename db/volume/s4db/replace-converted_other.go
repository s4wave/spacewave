//go:build !windows && !js && !wasip1

package volume_s4db

import "os"

// replaceConverted atomically replaces the active name while its old inode
// remains open under the source reader's writer lock.
func replaceConverted(from, to string) error {
	return os.Rename(from, to)
}
