//go:build !goscript

package block_store_filesegment

import "os"

// sameFile reports whether a and b describe the same file.
func sameFile(a, b os.FileInfo) bool {
	return os.SameFile(a, b)
}
