//go:build !windows

package s4dbv1

import "os"

// openSource opens the previous-format file for reading and writing, without
// creating or truncating it, so the in-place conversion can take the
// engine's write locks.
func openSource(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR, 0) // #nosec G703 -- the caller chooses the source.
}

// openSourceReadOnly opens the previous-format file only for reading.
func openSourceReadOnly(path string) (*os.File, error) {
	return os.Open(path) // #nosec G703 -- the caller chooses the source.
}
