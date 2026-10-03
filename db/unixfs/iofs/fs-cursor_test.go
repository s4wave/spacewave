package unixfs_iofs

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"path"
	"testing"
	"testing/fstest"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	iofs_mock "github.com/s4wave/spacewave/db/unixfs/iofs/mock"
	"github.com/sirupsen/logrus"
)

func TestFSCursor(t *testing.T) {
	// Prepare the context and logger for filesystem operations.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Open a cursor over the mock filesystem and retain its expected paths.
	ifs, expectedFiles := iofs_mock.NewMockIoFS()
	fsc, err := NewFSCursor(ifs)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Acquire a filesystem handle and release it after the test.
	handle, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	// Verify the filesystem adapter against the standard fs.FS contract.
	iofs := NewFS(ctx, handle)
	if err := fstest.TestFS(iofs, expectedFiles...); err != nil {
		t.Fatal(err.Error())
	}

	// test WithIgnorePath
	fph, _, err := handle.LookupPath(ctx, "testdir/testing.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fph.Release()

	// Verify that WithIgnorePath reads the selected file for an unrelated path.
	iofs = NewFS(ctx, fph, WithIgnorePath())
	data, err := iofs.ReadFile("foo/bar/baz/does/not/exist.zip")
	if err == nil && len(data) == 0 {
		err = errors.New("expected some file data with WithIgnorePath")
	}
	if err != nil {
		t.Fatal(err.Error())
	}
}

func TestFSFileReadSkipsReaderOnlyOffsetBeforeFinalBytes(t *testing.T) {
	// Open a reader-only filesystem containing a full chunk and trailing bytes.
	ctx := context.Background()
	const name = "dist/index.mjs"
	body := append(bytes.Repeat([]byte{'x'}, 32*1024), bytes.Repeat([]byte{'t'}, 105)...)
	fsc, err := NewFSCursor(readerOnlyMapFS{MapFS: fstest.MapFS{
		name: &fstest.MapFile{
			Data: body,
			Mode: 0o644,
		},
	}})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Acquire the root filesystem handle for path lookup.
	root, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer root.Release()

	// Look up the test file within the reader-only filesystem.
	fileHandle, _, err := root.LookupPath(ctx, name)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Wrap the file handle for sequential reads and release it after the test.
	file := &FSFile{ctx: ctx, handle: fileHandle}
	defer fileHandle.Release()

	// Read the first complete chunk from the file.
	first := make([]byte, 32*1024)
	n, err := file.Read(first)
	if err != nil {
		t.Fatalf("read first chunk: %v", err)
	}

	// Verify the first chunk length and contents.
	if n != len(first) {
		t.Fatalf("first chunk length = %d, want %d", n, len(first))
	}
	if !bytes.Equal(first, body[:len(first)]) {
		t.Fatal("first chunk data mismatch")
	}

	// Read the trailing bytes after the first chunk.
	second := make([]byte, 32*1024)
	n, err = file.Read(second)
	if err != nil {
		t.Fatalf("read final bytes: %v", err)
	}

	// Verify that the trailing bytes match the end of the file.
	if n != 105 {
		t.Fatalf("final read length = %d, want 105", n)
	}
	if !bytes.Equal(second[:n], body[32*1024:]) {
		t.Fatal("final read data mismatch")
	}

	// Verify that a subsequent read reports the end of the file.
	n, err = file.Read(second)
	if n != 0 || err != io.EOF {
		t.Fatalf("post-final read = %d, %v; want 0, EOF", n, err)
	}
}

type readerOnlyMapFS struct {
	fstest.MapFS
}

func (m readerOnlyMapFS) Open(name string) (fs.File, error) {
	mapFile, ok := m.MapFS[name]
	if !ok || mapFile.Mode.IsDir() {
		return m.MapFS.Open(name)
	}
	return &readerOnlyMapFile{
		reader: bytes.NewReader(mapFile.Data),
		info: readerOnlyMapFileInfo{
			name: name,
			file: mapFile,
		},
	}, nil
}

type readerOnlyMapFile struct {
	reader *bytes.Reader
	info   readerOnlyMapFileInfo
}

func (f *readerOnlyMapFile) Read(p []byte) (int, error) {
	return f.reader.Read(p)
}

func (f *readerOnlyMapFile) Stat() (fs.FileInfo, error) {
	return f.info, nil
}

func (f *readerOnlyMapFile) Close() error {
	return nil
}

type readerOnlyMapFileInfo struct {
	name string
	file *fstest.MapFile
}

func (f readerOnlyMapFileInfo) Name() string {
	return path.Base(f.name)
}

func (f readerOnlyMapFileInfo) Size() int64 {
	return int64(len(f.file.Data))
}

func (f readerOnlyMapFileInfo) Mode() fs.FileMode {
	return f.file.Mode
}

func (f readerOnlyMapFileInfo) ModTime() time.Time {
	return f.file.ModTime
}

func (f readerOnlyMapFileInfo) IsDir() bool {
	return f.file.Mode.IsDir()
}

func (f readerOnlyMapFileInfo) Sys() any {
	return f.file.Sys
}
