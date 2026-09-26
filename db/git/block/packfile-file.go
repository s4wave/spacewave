package git_block

import (
	"context"
	"io"
	"io/fs"
	"sync"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// packfileChunkCacheLimit bounds the chunk data one open pack retains. Every
// object lookup rereads the pack header in the first chunk and delta bases sit
// in other chunks, so random access revisits a small set of chunks.
const packfileChunkCacheLimit = 8 << 20

// PackfileFile exposes an immutable chunked blob as a seekable Git packfile.
// Reads retain at most packfileChunkCacheLimit bytes of chunk data. Close joins
// pending reads before the caller releases the enclosing block transaction.
type PackfileFile struct {
	mtx    sync.Mutex
	name   string
	reader *blob.Reader
	size   int64
	closed bool
}

// NewPackfileFile opens a read-only file at a blob cursor owned by the caller.
func NewPackfileFile(ctx context.Context, name string, cursor *block.Cursor) (*PackfileFile, error) {
	reader, err := blob.NewReader(ctx, cursor)
	if err != nil {
		return nil, err
	}
	reader.SetChunkCacheLimit(packfileChunkCacheLimit)
	size, err := reader.Seek(0, io.SeekEnd)
	if err != nil {
		_ = reader.Close()
		return nil, err
	}
	if _, err := reader.Seek(0, io.SeekStart); err != nil {
		_ = reader.Close()
		return nil, err
	}
	return &PackfileFile{name: name, reader: reader, size: size}, nil
}

// Name returns the immutable packfile name.
func (f *PackfileFile) Name() string { return f.name }

// Read advances the sequential file position.
func (f *PackfileFile) Read(p []byte) (int, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.closed {
		return 0, fs.ErrClosed
	}
	return f.reader.Read(p)
}

// ReadAt reads a complete range without changing the sequential file position.
func (f *PackfileFile) ReadAt(p []byte, off int64) (int, error) {
	// Serialize the seek/read/restore sequence with all other file operations.
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.closed {
		return 0, fs.ErrClosed
	}
	position, err := f.reader.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	if _, err := f.reader.Seek(off, io.SeekStart); err != nil {
		return 0, err
	}

	// A short positional read reports EOF, including a partial final chunk.
	n, readErr := io.ReadFull(f.reader, p)
	_, restoreErr := f.reader.Seek(position, io.SeekStart)
	if readErr == io.ErrUnexpectedEOF {
		readErr = io.EOF
	}
	if readErr != nil {
		return n, readErr
	}
	return n, restoreErr
}

// Seek changes the sequential file position without reading intervening chunks.
func (f *PackfileFile) Seek(offset int64, whence int) (int64, error) {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.closed {
		return 0, fs.ErrClosed
	}
	if whence != io.SeekStart && whence != io.SeekCurrent && whence != io.SeekEnd {
		return 0, errors.New("invalid seek whence")
	}
	return f.reader.Seek(offset, whence)
}

// Write rejects mutations to the immutable stored packfile.
func (f *PackfileFile) Write([]byte) (int, error) { return 0, fs.ErrPermission }

// WriteAt rejects mutations to the immutable stored packfile.
func (f *PackfileFile) WriteAt([]byte, int64) (int, error) { return 0, fs.ErrPermission }

// Truncate rejects mutations to the immutable stored packfile.
func (f *PackfileFile) Truncate(int64) error { return fs.ErrPermission }

// Close releases buffered chunks and joins the reader exactly once.
func (f *PackfileFile) Close() error {
	f.mtx.Lock()
	defer f.mtx.Unlock()
	if f.closed {
		return nil
	}
	f.closed = true
	return f.reader.Close()
}

// Stat returns immutable file metadata without reading pack contents.
func (f *PackfileFile) Stat() (fs.FileInfo, error) {
	return packfileInfo{name: f.name, size: f.size}, nil
}

type packfileInfo struct {
	name string
	size int64
}

func (i packfileInfo) Name() string       { return i.name }
func (i packfileInfo) Size() int64        { return i.size }
func (i packfileInfo) Mode() fs.FileMode  { return 0o444 }
func (i packfileInfo) ModTime() time.Time { return time.Time{} }
func (i packfileInfo) IsDir() bool        { return false }
func (i packfileInfo) Sys() any           { return nil }

var _ billy.File = (*PackfileFile)(nil)
