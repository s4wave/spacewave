//go:build !linux && !darwin

package common

import "os"

// syncFileData flushes a file's data and metadata.
func syncFileData(f *os.File) error {
	return f.Sync()
}

// writeBackFile does nothing here: the platform's only flush is durable and
// would cost a pass more than it saves the flush under the writer turn.
func writeBackFile(*os.File) error {
	return nil
}
