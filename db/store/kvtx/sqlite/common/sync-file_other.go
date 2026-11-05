//go:build !linux

package common

import "os"

// syncFileData flushes a file's data and metadata.
func syncFileData(f *os.File) error {
	return f.Sync()
}
