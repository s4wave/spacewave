package unixfs_empty

import (
	"context"
	"testing"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_errors "github.com/aperturerobotics/hydra/unixfs/errors"
	"github.com/sirupsen/logrus"
)

func TestFSCursor(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	fsc := NewFSCursor()

	handle, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	fph, _, err := handle.LookupPath(ctx, "testdir/testing.txt")
	if fph != nil {
		fph.Release()
	}
	if err != unixfs_errors.ErrNotExist {
		t.Fatal(err.Error())
	}
}
