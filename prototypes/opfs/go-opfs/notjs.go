//go:build !js || !wasm

// Package opfs provides Go wrappers for the Origin Private File System browser API.
//
// This file provides stub declarations so the package compiles on non-WASM
// platforms. All functions return errors indicating OPFS is unavailable.
package opfs

import (
	"errors"
	"io"
	"io/fs"
	"time"
)

var errNotAvailable = errors.New("opfs: not available outside js/wasm")

// DirectoryHandle is a stub for non-WASM platforms.
type DirectoryHandle struct{}

// FileHandle is a stub for non-WASM platforms.
type FileHandle struct {
	name string
}

// SyncAccessHandle is a stub for non-WASM platforms.
type SyncAccessHandle struct {
	name   string
	cursor int64
}

// Entry represents a directory entry.
type Entry struct {
	Name string
	Kind string
}

// AsFileHandle is a stub that always returns nil.
func (e *Entry) AsFileHandle() *FileHandle { return nil }

// AsDirectoryHandle is a stub that always returns nil.
func (e *Entry) AsDirectoryHandle() *DirectoryHandle { return nil }

// DOMError represents a JavaScript DOMException.
type DOMError struct {
	Message string
}

// Error implements the error interface.
func (e *DOMError) Error() string { return e.Message }

// GetRootDirectory is a stub that returns an error on non-WASM platforms.
func GetRootDirectory() (*DirectoryHandle, error) {
	return nil, errNotAvailable
}

// GetDirectoryHandle is a stub.
func (dh *DirectoryHandle) GetDirectoryHandle(name string, create bool) (*DirectoryHandle, error) {
	return nil, errNotAvailable
}

// GetFileHandle is a stub.
func (dh *DirectoryHandle) GetFileHandle(name string, create bool) (*FileHandle, error) {
	return nil, errNotAvailable
}

// RemoveEntry is a stub.
func (dh *DirectoryHandle) RemoveEntry(name string, recursive bool) error {
	return errNotAvailable
}

// Entries is a stub.
func (dh *DirectoryHandle) Entries() ([]Entry, error) {
	return nil, errNotAvailable
}

// CreateSyncAccessHandle is a stub.
func (fh *FileHandle) CreateSyncAccessHandle() (*SyncAccessHandle, error) {
	return nil, errNotAvailable
}

// ReadFile is a stub. Returns an error on non-WASM platforms.
func (fh *FileHandle) ReadFile() ([]byte, error) {
	return nil, errNotAvailable
}

// Read is a stub.
func (h *SyncAccessHandle) Read(buf []byte) (int, error) {
	return 0, errNotAvailable
}

// ReadAt is a stub.
func (h *SyncAccessHandle) ReadAt(buf []byte, off int64) (int, error) {
	return 0, errNotAvailable
}

// Write is a stub.
func (h *SyncAccessHandle) Write(buf []byte) (int, error) {
	return 0, errNotAvailable
}

// WriteAt is a stub.
func (h *SyncAccessHandle) WriteAt(buf []byte, off int64) (int, error) {
	return 0, errNotAvailable
}

// Seek is a stub.
func (h *SyncAccessHandle) Seek(offset int64, whence int) (int64, error) {
	return 0, errNotAvailable
}

// Stat is a stub.
func (h *SyncAccessHandle) Stat() (fs.FileInfo, error) {
	return nil, errNotAvailable
}

// Truncate is a stub.
func (h *SyncAccessHandle) Truncate(size int64) error {
	return errNotAvailable
}

// Flush is a stub.
func (h *SyncAccessHandle) Flush() error {
	return errNotAvailable
}

// Close is a stub.
func (h *SyncAccessHandle) Close() error {
	return errNotAvailable
}

// FileOps provides file-like access to an OPFS file.
type FileOps interface {
	fs.File
	io.Writer
	io.Seeker
	io.ReaderAt
	io.WriterAt
	Truncate(size int64) error
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

// ModTime returns zero time.
func (fi *fileInfo) ModTime() time.Time { return time.Time{} }

// IsDir returns false.
func (fi *fileInfo) IsDir() bool { return false }

// Sys returns nil.
func (fi *fileInfo) Sys() any { return nil }

// SyncSupported is a stub that returns false on non-WASM platforms.
func SyncSupported() bool { return false }

// OpenFileOps is a stub.
func (fh *FileHandle) OpenFileOps() (FileOps, error) {
	return nil, errNotAvailable
}
