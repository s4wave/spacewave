package unixfs_access

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	billy_util "github.com/go-git/go-billy/v5/util"
)

func TestAccessFsCursor(t *testing.T) {
	ctx := context.Background()
	objKey := "test-fs"
	fs, tb, err := unixfs_world.BuildTestbed(
		ctx,
		objKey,
		true,
		testbed.WithVerbose(true),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// fill the sample filesystem
	rootRef, err := fs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rootRef.Release()

	rbfs := unixfs.NewBillyFS(ctx, rootRef, "", time.Now())
	testData := []byte("hello world")
	if err := billy_util.WriteFile(rbfs, "/bat/baz/test-file.txt", testData, 0755); err != nil {
		t.Fatal(err.Error())
	}

	// wait a moment for the write to be confirmed
	// TODO: This is a bug that currently is being fixed
	<-time.After(time.Millisecond * 100)

	// test accessing with access cursor
	fsCursor := NewFSCursor(NewAccessUnixFSFunc(fs))
	accessFs := unixfs.NewFS(ctx, tb.Logger, fsCursor, []string{"bat"})
	fsHandle, err := accessFs.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	fsHandleAfs := unixfs.NewAferoFS(ctx, fsHandle, "/bat/", time.Now())
	fi, err := fsHandleAfs.Stat("baz/test-file.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.Logger.Infof("successfully stat() via FSCursor: %s", fi.Name())
	// fsHandle := unixfs.newfs
}
