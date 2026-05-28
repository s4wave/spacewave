package space_unixfs

import (
	"context"
	"io"
	"testing"
	"time"

	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// setupFSCursorTestbed constructs a hydra+world testbed and returns the
// pieces every fs-cursor test needs: the lifetime context, a debug logger
// entry, and the world testbed. The world testbed is released via t.Cleanup.
func setupFSCursorTestbed(t *testing.T) (context.Context, *logrus.Entry, *world_testbed.Testbed) {
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)
	return ctx, le, wtb
}

// addUnixFSLookupController registers a lookup-op controller for unixfs ops on
// the given world testbed. Helper for tests that drive unixfs projections.
func addUnixFSLookupController(t *testing.T, ctx context.Context, wtb *world_testbed.Testbed, name string) {
	t.Helper()
	opc := world.NewLookupOpController(name, wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, opc, nil); err != nil {
		t.Fatal(err)
	}
}

func TestFSCursorProjectsUnixFSObjectPaths(t *testing.T) {
	ctx, le, wtb := setupFSCursorTestbed(t)
	addUnixFSLookupController(t, ctx, wtb, "test-space-projection")

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	objectKey := "docs/demo"
	if _, _, err := unixfs_world.FsInit(
		ctx,
		ws,
		sender,
		objectKey,
		unixfs_world.FSType_FSType_FS_NODE,
		nil,
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	tx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	objectCursor, _ := unixfs_world.NewFSCursorWithWriter(
		ctx,
		le,
		tx,
		objectKey,
		unixfs_world.FSType_FSType_FS_NODE,
		sender,
	)
	if err != nil {
		t.Fatal(err)
	}
	objectHandle, err := unixfs.NewFSHandle(objectCursor)
	if err != nil {
		objectCursor.Release()
		t.Fatal(err)
	}
	defer objectHandle.Release()

	if err := objectHandle.MkdirAll(ctx, []string{"nested"}, 0o755, time.Now()); err != nil {
		t.Fatal(err)
	}
	nestedHandle, _, err := objectHandle.LookupPath(ctx, "nested")
	if err != nil {
		t.Fatal(err)
	}
	if err := nestedHandle.Mknod(ctx, true, []string{"hello.txt"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Now()); err != nil {
		nestedHandle.Release()
		t.Fatal(err)
	}
	nestedHandle.Release()
	fileHandle, _, err := objectHandle.LookupPath(ctx, "nested/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileHandle.WriteAt(ctx, 0, []byte("hello world"), time.Now()); err != nil {
		fileHandle.Release()
		t.Fatal(err)
	}
	fileHandle.Release()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 7, "space-1")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	projectedFile, _, err := rootHandle.LookupPath(ctx, "u/7/so/space-1/-/docs/demo/-/nested/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer projectedFile.Release()

	buf := make([]byte, 32)
	n, err := projectedFile.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestFSCursorDisambiguatesObjectKeyAndDescendantPaths(t *testing.T) {
	ctx, le, wtb := setupFSCursorTestbed(t)
	addUnixFSLookupController(t, ctx, wtb, "test-space-projection-overlap")

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	now := time.Now()

	writeObjectFile := func(objectKey, name, content string) {
		if _, _, err := unixfs_world.FsInit(
			ctx,
			ws,
			sender,
			objectKey,
			unixfs_world.FSType_FSType_FS_NODE,
			nil,
			true,
			now,
		); err != nil {
			t.Fatal(err)
		}

		tx, err := wtb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()

		cursor, _ := unixfs_world.NewFSCursorWithWriter(
			ctx,
			le,
			tx,
			objectKey,
			unixfs_world.FSType_FSType_FS_NODE,
			sender,
		)
		handle, err := unixfs.NewFSHandle(cursor)
		if err != nil {
			cursor.Release()
			t.Fatal(err)
		}
		defer handle.Release()

		if err := handle.Mknod(ctx, true, []string{name}, unixfs.NewFSCursorNodeType_File(), 0o644, now); err != nil {
			t.Fatal(err)
		}
		fileHandle, _, err := handle.LookupPath(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		if err := fileHandle.WriteAt(ctx, 0, []byte(content), now); err != nil {
			fileHandle.Release()
			t.Fatal(err)
		}
		fileHandle.Release()
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	writeObjectFile("foo/bar", "hello.txt", "object one")
	writeObjectFile("foo/bar/files", "root.txt", "object two")

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 9, "space-9")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	direntNames := make([]string, 0, 2)
	fooBarHandle, _, err := rootHandle.LookupPath(ctx, "u/9/so/space-9/-/foo/bar")
	if err != nil {
		t.Fatal(err)
	}
	if err := fooBarHandle.ReaddirAll(ctx, 0, func(ent unixfs.FSCursorDirent) error {
		direntNames = append(direntNames, ent.GetName())
		return nil
	}); err != nil {
		fooBarHandle.Release()
		t.Fatal(err)
	}
	fooBarHandle.Release()

	if len(direntNames) != 2 || direntNames[0] != "-" || direntNames[1] != "files" {
		t.Fatalf("unexpected foo/bar projection children: %#v", direntNames)
	}

	firstHandle, _, err := rootHandle.LookupPath(ctx, "u/9/so/space-9/-/foo/bar/-/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer firstHandle.Release()

	secondHandle, _, err := rootHandle.LookupPath(ctx, "u/9/so/space-9/-/foo/bar/files/-/root.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer secondHandle.Release()

	buf := make([]byte, 32)
	n, err := firstHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "object one" {
		t.Fatalf("got first %q, want %q", got, "object one")
	}

	n, err = secondHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "object two" {
		t.Fatalf("got second %q, want %q", got, "object two")
	}
}
