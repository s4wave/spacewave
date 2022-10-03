package iofs_mock

import (
	"io/fs"
	"testing/fstest"
	"time"
)

// NewMockIoFS constructs a mock fs.FS.
func NewMockIoFS() (fs.FS, []string) {
	mfs := make(fstest.MapFS)
	baseTime := time.Now()
	var expected []string
	addFile := func(fpath string, data []byte) {
		expected = append(expected, fpath)
		mfs[fpath] = &fstest.MapFile{
			Data:    data,
			ModTime: baseTime,
		}
	}
	addFile("test.txt", []byte("hello world"))
	addFile("testdir/testing.txt", []byte("file within a directory"))
	return mfs, expected
}
