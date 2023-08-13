package unixfs_iofs

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/aperturerobotics/hydra/unixfs"
	iofs_mock "github.com/aperturerobotics/hydra/unixfs/iofs/mock"
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

	iofs := NewFS(ctx, handle)
	if err := fstest.TestFS(iofs, expectedFiles...); err != nil {
		t.Fatal(err.Error())
	}
}
