package space_http_export

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
)

func TestWalkAndZipUnixFS(t *testing.T) {
	ctx := context.Background()
	mfs := fstest.MapFS{
		"hello.txt":         {Data: []byte("hello world")},
		"subdir/nested.txt": {Data: []byte("nested content")},
		"subdir/deep/a.txt": {Data: []byte("deep file")},
		"empty-dir":         {Mode: 0o755 | fs.ModeDir},
	}

	cursor, err := unixfs_iofs.NewFSCursor(mfs)
	if err != nil {
		t.Fatal(err)
	}
	fsh, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		cursor.Release()
		t.Fatal(err)
	}
	defer fsh.Release()

	var buf bytes.Buffer
	if err := exportZip(ctx, &buf, fsh); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}

	entries := make(map[string]*zip.File)
	for _, f := range zr.File {
		entries[f.Name] = f
	}
	expectedFiles := map[string]string{
		"hello.txt":         "hello world",
		"subdir/nested.txt": "nested content",
		"subdir/deep/a.txt": "deep file",
	}
	for name, wantContent := range expectedFiles {
		f, ok := entries[name]
		if !ok {
			t.Errorf("missing expected file: %s", name)
			continue
		}
		rc, err := f.Open()
		if err != nil {
			t.Errorf("open %s: %v", name, err)
			continue
		}
		data, err := io.ReadAll(rc)
		if closeErr := rc.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		if string(data) != wantContent {
			t.Errorf("%s: got %q, want %q", name, data, wantContent)
		}
	}

	for _, name := range []string{"subdir/", "subdir/deep/"} {
		if _, ok := entries[name]; !ok {
			t.Errorf("missing expected directory entry: %s", name)
		}
	}
}
