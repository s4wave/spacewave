//go:build goscript

package block_store_filesegment

import "os"

// sameFile reports true: GoScript's FileInfo carries no device or inode, and
// its os.SameFile reports false for every pair. checkUnchanged then detects a
// replaced file by its size and modification time alone.
func sameFile(a, b os.FileInfo) bool {
	return true
}
