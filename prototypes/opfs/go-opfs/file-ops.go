//go:build js && wasm

package opfs

import (
	"io"
	"io/fs"
	"time"

	"github.com/hack-pad/safejs"
)

// FileOps provides file-like access to an OPFS file.
// Extends fs.File with write, seek, and OPFS-specific operations.
// The sync variant uses FileSystemSyncAccessHandle (workers only).
// The async variant uses getFile/createWritable (all contexts).
type FileOps interface {
	fs.File     // Stat, Read, Close
	io.Writer   // Write
	io.Seeker   // Seek
	io.ReaderAt // ReadAt
	io.WriterAt // WriteAt

	// Truncate resizes the file to the given size.
	Truncate(size int64) error
	// Flush persists any buffered writes.
	Flush() error
}

// fileInfo implements fs.FileInfo backed by a size value.
type fileInfo struct {
	name string
	size int64
}

// Name returns the base name of the file.
func (fi *fileInfo) Name() string { return fi.name }

// Size returns the file size in bytes.
func (fi *fileInfo) Size() int64 { return fi.size }

// Mode returns regular file mode.
func (fi *fileInfo) Mode() fs.FileMode { return 0o644 }

// ModTime returns zero time (OPFS does not expose modification time).
func (fi *fileInfo) ModTime() time.Time { return time.Time{} }

// IsDir returns false.
func (fi *fileInfo) IsDir() bool { return false }

// Sys returns nil.
func (fi *fileInfo) Sys() any { return nil }

// syncSupported caches the result of sync API detection.
var syncSupported *bool

// SyncSupported returns true if FileSystemSyncAccessHandle is available
// in the current context. The result is cached after the first call.
func SyncSupported() bool {
	if syncSupported != nil {
		return *syncSupported
	}
	supported := detectSyncSupport()
	syncSupported = &supported
	return supported
}

// detectSyncSupport probes for createSyncAccessHandle availability by
// checking if the method exists on FileSystemFileHandle.prototype.
func detectSyncSupport() bool {
	proto, err := safejs.Global().Get("FileSystemFileHandle")
	if err != nil {
		return false
	}
	truthy, err := proto.Truthy()
	if err != nil || !truthy {
		return false
	}
	pt, err := proto.Get("prototype")
	if err != nil {
		return false
	}
	method, err := pt.Get("createSyncAccessHandle")
	if err != nil {
		return false
	}
	t, err := method.Truthy()
	if err != nil {
		return false
	}
	return t
}

// OpenFileOps opens a FileOps for the given FileHandle.
// Uses SyncAccessHandle if available, otherwise falls back to async.
func (fh *FileHandle) OpenFileOps() (FileOps, error) {
	if SyncSupported() {
		sh, err := fh.CreateSyncAccessHandle()
		if err != nil {
			// Sync detection said yes but open failed (e.g. file locked).
			// Fall back to async.
			return newAsyncFileOps(fh)
		}
		return sh, nil
	}
	return newAsyncFileOps(fh)
}

// Verify SyncAccessHandle implements FileOps.
var _ FileOps = (*SyncAccessHandle)(nil)
