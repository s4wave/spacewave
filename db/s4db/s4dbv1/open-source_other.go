//go:build !windows

package s4dbv1

import "os"

// openSource opens the previous-format file without creating or truncating it.
func openSource(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDWR, 0) // #nosec G703 -- the caller chooses the source.
}
