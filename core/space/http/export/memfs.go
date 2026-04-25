package space_http_export

import (
	"bytes"
	"io"
	"io/fs"
	"time"
)

// memFS is a minimal read-only in-memory fs.FS containing a single file.
type memFS struct {
	name string
	data []byte
}

// Open implements fs.FS.
func (m *memFS) Open(name string) (fs.File, error) {
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	if name == "." {
		return &memDir{fs: m}, nil
	}
	if name == m.name {
		return &memFile{name: m.name, reader: bytes.NewReader(m.data), size: int64(len(m.data))}, nil
	}
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}

// ReadDir implements fs.ReadDirFS.
func (m *memFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name != "." {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrNotExist}
	}
	return []fs.DirEntry{&memDirEntry{name: m.name, size: int64(len(m.data))}}, nil
}

// Stat implements fs.StatFS.
func (m *memFS) Stat(name string) (fs.FileInfo, error) {
	if name == "." {
		return &memFileInfo{name: ".", isDir: true}, nil
	}
	if name == m.name {
		return &memFileInfo{name: m.name, size: int64(len(m.data))}, nil
	}
	return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
}

// memFile is an in-memory file implementing fs.File and io.ReaderAt.
type memFile struct {
	name   string
	reader *bytes.Reader
	size   int64
}

// Stat implements fs.File.
func (f *memFile) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: f.name, size: f.size}, nil
}

// Read implements fs.File.
func (f *memFile) Read(b []byte) (int, error) {
	return f.reader.Read(b)
}

// ReadAt implements io.ReaderAt.
func (f *memFile) ReadAt(b []byte, off int64) (int, error) {
	return f.reader.ReadAt(b, off)
}

// Close implements fs.File.
func (f *memFile) Close() error {
	return nil
}

// memDir is a directory entry for the root.
type memDir struct {
	fs   *memFS
	read bool
}

// Stat implements fs.File.
func (d *memDir) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: ".", isDir: true}, nil
}

// Read implements fs.File.
func (d *memDir) Read([]byte) (int, error) {
	return 0, io.EOF
}

// Close implements fs.File.
func (d *memDir) Close() error {
	return nil
}

// ReadDir implements fs.ReadDirFile.
func (d *memDir) ReadDir(n int) ([]fs.DirEntry, error) {
	if d.read {
		return nil, io.EOF
	}
	d.read = true
	entries := []fs.DirEntry{&memDirEntry{name: d.fs.name, size: int64(len(d.fs.data))}}
	if n > 0 {
		return entries, io.EOF
	}
	return entries, nil
}

// memFileInfo implements fs.FileInfo.
type memFileInfo struct {
	name  string
	size  int64
	isDir bool
}

func (fi *memFileInfo) Name() string { return fi.name }
func (fi *memFileInfo) Size() int64  { return fi.size }
func (fi *memFileInfo) Mode() fs.FileMode {
	if fi.isDir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (fi *memFileInfo) ModTime() time.Time { return time.Time{} }
func (fi *memFileInfo) IsDir() bool        { return fi.isDir }
func (fi *memFileInfo) Sys() any           { return nil }

// memDirEntry implements fs.DirEntry.
type memDirEntry struct {
	name string
	size int64
}

func (de *memDirEntry) Name() string      { return de.name }
func (de *memDirEntry) IsDir() bool       { return false }
func (de *memDirEntry) Type() fs.FileMode { return 0 }
func (de *memDirEntry) Info() (fs.FileInfo, error) {
	return &memFileInfo{name: de.name, size: de.size}, nil
}
