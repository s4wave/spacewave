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
	// Create the context and debug logger for the filesystem testbed.
	t.Helper()
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed for the projected World.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World engine and register its test cleanup.
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
	// Start the World testbed with UnixFS operations enabled.
	ctx, le, wtb := setupFSCursorTestbed(t)
	addUnixFSLookupController(t, ctx, wtb, "test-space-projection")

	// Initialize the UnixFS object to project beneath the shared object path.
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

	// Open a writable World transaction for the object contents.
	tx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Open a writable filesystem handle on the UnixFS object.
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

	// Create a nested directory and its empty hello.txt file.
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

	// Write hello.txt through its filesystem handle.
	fileHandle, _, err := objectHandle.LookupPath(ctx, "nested/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileHandle.WriteAt(ctx, 0, []byte("hello world"), time.Now()); err != nil {
		fileHandle.Release()
		t.Fatal(err)
	}
	fileHandle.Release()

	// Commit the UnixFS object contents to the World.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Open the projected filesystem root and retain its handle.
	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 7, "space-1")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Open hello.txt through the synthetic session and shared object path.
	projectedFile, _, err := rootHandle.LookupPath(ctx, "u/7/so/space-1/-/docs/demo/-/nested/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer projectedFile.Release()

	// Read hello.txt through its projected filesystem handle.
	buf := make([]byte, 32)
	n, err := projectedFile.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Verify that the projected file returns the stored contents.
	if got := string(buf[:n]); got != "hello world" {
		t.Fatalf("got %q, want %q", got, "hello world")
	}
}

func TestFSCursorDisambiguatesObjectKeyAndDescendantPaths(t *testing.T) {
	// Start the World testbed with UnixFS operations enabled.
	ctx, le, wtb := setupFSCursorTestbed(t)
	addUnixFSLookupController(t, ctx, wtb, "test-space-projection-overlap")

	// Prepare the writable World and timestamp for overlapping object paths.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	now := time.Now()

	// Define a writer that commits one file into each UnixFS object.
	writeObjectFile := func(objectKey, name, content string) {
		// Initialize the UnixFS object for the requested object key.
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

		// Open a writable World transaction for the object contents.
		tx, err := wtb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()

		// Open a writable filesystem handle on the initialized object.
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

		// Create and write the requested file inside the UnixFS object.
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

		// Commit the file contents to the World.
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Populate both the parent object and its descendant object.
	writeObjectFile("foo/bar", "hello.txt", "object one")
	writeObjectFile("foo/bar/files", "root.txt", "object two")

	// Open the projected filesystem root and retain its handle.
	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 9, "space-9")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	// Read the projected children at the overlapping parent object path.
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

	// Verify that the parent exposes its mount marker and descendant directory.
	if len(direntNames) != 2 || direntNames[0] != "-" || direntNames[1] != "files" {
		t.Fatalf("unexpected foo/bar projection children: %#v", direntNames)
	}

	// Open the file mounted at the parent object path.
	firstHandle, _, err := rootHandle.LookupPath(ctx, "u/9/so/space-9/-/foo/bar/-/hello.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer firstHandle.Release()

	// Open the file mounted at the descendant object path.
	secondHandle, _, err := rootHandle.LookupPath(ctx, "u/9/so/space-9/-/foo/bar/files/-/root.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer secondHandle.Release()

	// Read the file belonging to the parent object.
	buf := make([]byte, 32)
	n, err := firstHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Verify that the parent mount returns its own file contents.
	if got := string(buf[:n]); got != "object one" {
		t.Fatalf("got first %q, want %q", got, "object one")
	}

	// Read the file belonging to the descendant object.
	n, err = secondHandle.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}

	// Verify that the descendant mount returns its own file contents.
	if got := string(buf[:n]); got != "object two" {
		t.Fatalf("got second %q, want %q", got, "object two")
	}
}
