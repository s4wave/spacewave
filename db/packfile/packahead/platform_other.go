//go:build !linux && !darwin

package packahead

import "os"

// syncData makes the file's written data durable on the drive.
func syncData(f *os.File) error {
	return f.Sync()
}
