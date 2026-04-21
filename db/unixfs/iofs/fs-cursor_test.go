package unixfs_iofs

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/s4wave/spacewave/db/unixfs"
	iofs_mock "github.com/s4wave/spacewave/db/unixfs/iofs/mock"
	"github.com/sirupsen/logrus"
)

func TestFSCursor(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	ifs, expectedFiles := iofs_mock.NewMockIoFS()
	fsc, err := NewFSCursor(ifs)
	if err != nil {
		t.Fatal(err.Error())
	}

	handle, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

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

	iofs = NewFS(ctx, fph, WithIgnorePath())
	data, err := iofs.ReadFile("foo/bar/baz/does/not/exist.zip")
	if err == nil && len(data) == 0 {
		err = errors.New("expected some file data with WithIgnorePath")
	}
	if err != nil {
		t.Fatal(err.Error())
	}
}
