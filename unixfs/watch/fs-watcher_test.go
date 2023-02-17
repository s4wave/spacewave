package unixfs_watch

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	unixfs_world_access "github.com/aperturerobotics/hydra/unixfs/world/access"
	billy_util "github.com/go-git/go-billy/v5/util"
)

func TestFSWatcher(t *testing.T) {
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

	// construct the AccessUnixFS handler
	unixFsID := "test-fs"
	accessCtrl, err := unixfs_world_access.NewController(
		tb.Logger,
		tb.Bus,
		&unixfs_world_access.Config{
			FsId:   unixFsID,
			FsRef:  &unixfs_world.UnixfsRef{ObjectKey: objKey},
			PeerId: tb.Volume.GetPeerID().Pretty(),
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	accessRel, err := tb.Bus.AddController(ctx, accessCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer accessRel()

	// access it!
	accessUfs, ufsRef, err := unixfs_access.ExAccessUnixFS(ctx, tb.Bus, unixFsID, false, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ufsRef.Release()

	// the callback is called whenever the fs changes
	callbackCalled := make(chan error, 1)
	fsWatcherCb := func(
		ctx context.Context,
		fsTargetPath []string,
		fsError error,
		fsPath []string,
		fsHandle *unixfs.FSHandle,
		fsCursor unixfs.FSCursor,
		fsOps unixfs.FSCursorOps,
	) error {
		tb.Logger.Infof("fs-watcher: callback called: %v, %v", fsError, fsPath)
		select {
		case callbackCalled <- fsError:
		default:
		}
		return nil
	}

	// construct the FSWatcher
	watcher := NewFSWatcher(fsWatcherCb, accessUfs)

	// execute the FSWatcher
	go func() {
		err := watcher.Execute(tb.Context, nil)
		if err != nil {
			select {
			case callbackCalled <- err:
			default:
			}
		}
	}()

	// assert that the callback is not called yet
	assertNotCalled := func() {
		select {
		case <-callbackCalled:
			t.Fail()
		case <-time.After(time.Millisecond * 100):
		}
	}
	assertNotCalled()

	// set the path
	watcher.SetPath("/bat/baz")

	// assert that the callback is called
	assertCalled := func() {
		select {
		case err := <-callbackCalled:
			if err != nil {
				t.Fatal(err.Error())
			}
		case <-time.After(time.Millisecond * 100):
			t.Fatal("timeout waiting for fs watcher to call callback")
		}
	}
	assertCalled()
	// assert that the callback is not called twice
	assertNotCalled()

	// change the fs
	handle, handleRel, err := accessUfs(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	handleBfs := unixfs.NewBillyFS(ctx, handle, "", time.Now())
	if err := billy_util.WriteFile(handleBfs, "bat/baz/testing2.txt", []byte("test file #2\n"), 0644); err != nil {
		t.Fatal(err.Error())
	}
	handleRel()
	// wait a moment for the write to be confirmed
	// TODO: This is a bug that currently is being fixed
	<-time.After(time.Millisecond * 100)

	// assert that the callback is called again
	assertCalled()
	// assert that the callback is not called twice
	assertNotCalled()
}
