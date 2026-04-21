package unixfs_world_testbed

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_e2e "github.com/s4wave/spacewave/db/unixfs/e2e"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

var objKey = "test/fs"

// TestFs runs the e2e tests.
func TestFs(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	watchWorldChanges := true
	fsHandle, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	if err := unixfs_e2e.TestUnixFS(ctx, fsHandle); err != nil {
		t.Fatal(err.Error())
	}
}

// TestFs_SingleTxn runs the e2e tests with a single transaction.
func TestFs_SingleTxn(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	tb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// provide op handlers to bus
	engineID := tb.EngineID
	opc := world.NewLookupOpController("test-fs-ops", engineID, unixfs_world.LookupFsOp)
	_, err = tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// uses directive to look up the engine
	eng := tb.Engine
	sender := tb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE

	// init the fs
	if err := func() error {
		// build a write txn
		wtx, err := eng.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer wtx.Discard()

		typeID, _ := unixfs_world.FSTypeToTypeID(fsType)
		_, _, err = unixfs_world.FsInit(
			ctx,
			wtx,
			sender,
			objKey,
			fsType,
			nil,
			true,
			time.Now(),
		)
		if err != nil {
			return err
		}

		// check type
		if err := world_types.CheckObjectType(ctx, wtx, objKey, typeID); err != nil {
			return err
		}

		return wtx.Commit(ctx)
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// construct full fs
	tb.Logger.Debug("filesystem initialized")

	buildFsh := func() (wtx world.Tx, fsh *unixfs.FSHandle, err error) {
		wtx, err = eng.NewTransaction(ctx, true)
		if err != nil {
			return nil, nil, err
		}

		fsCursor, _ := unixfs_world.NewFSCursorWithWriter(ctx, le, wtx, objKey, fsType, sender)
		fsh, err = unixfs.NewFSHandle(fsCursor)
		if err != nil {
			wtx.Discard()
			fsCursor.Release()
			return nil, nil, err
		}

		return wtx, fsh, nil
	}

	// quick test using a temporary (not written) txn
	// we expect to be able to do everything on a temporary fs txn without committing
	if err := func() error {
		wtx, fsh, err := buildFsh()
		if err != nil {
			return err
		}
		defer wtx.Discard()
		defer fsh.Release()

		if err := fsh.Mknod(ctx, false, []string{"mydir"}, unixfs.NewFSCursorNodeType_Dir(), 0o644, time.Now()); err != nil {
			return err
		}

		if err := fsh.MkdirAll(ctx, []string{"test", "dir"}, 0o700, time.Now()); err != nil {
			return err
		}

		if err := fsh.Mknod(ctx, false, []string{"hello.txt", "world.md"}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Now()); err != nil {
			return err
		}

		// success
		return wtx.Commit(ctx)
	}(); err != nil {
		t.Fatal(err.Error())
	}

	// full test on write txn with commit
	// we expect to be able to do everything on a temporary fs txn without committing
	if err := func() error {
		wtx, fsh, err := buildFsh()
		if err != nil {
			return err
		}
		defer wtx.Discard()
		defer fsh.Release()

		if err := unixfs_e2e.TestUnixFS(ctx, fsh); err != nil {
			return err
		}

		// success
		return wtx.Commit(ctx)
	}(); err != nil {
		t.Fatal(err.Error())
	}
}

// TestMknodWithContent tests creating a file with content atomically.
func TestMknodWithContent(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	watchWorldChanges := true
	fsHandle, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// create a file with content using MknodWithContent
	content := []byte("Hello, MknodWithContent! This is a test file with atomic content.")
	err = fsHandle.MknodWithContent(
		ctx,
		"test-file.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		bytes.NewReader(content),
		0o644,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// verify: look up the file and read it back
	fileHandle, err := fsHandle.Lookup(ctx, "test-file.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fileHandle.Release()

	// check file size
	size, err := fileHandle.GetSize(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if size != uint64(len(content)) {
		t.Fatalf("expected size %d but got %d", len(content), size)
	}

	// read file content
	buf := make([]byte, len(content))
	n, err := fileHandle.ReadAt(ctx, 0, buf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if int(n) != len(content) {
		t.Fatalf("expected to read %d bytes but got %d", len(content), n)
	}
	if !bytes.Equal(buf[:n], content) {
		t.Fatalf("content mismatch: %q != %q", buf[:n], content)
	}
}

// TestMknodWithContent_LargeFile tests creating a larger file that requires chunking.
func TestMknodWithContent_LargeFile(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	watchWorldChanges := true
	fsHandle, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// create a large file (2MB -- above the raw blob high water mark)
	content := []byte(strings.Repeat("abcdefghij", 200000))
	err = fsHandle.MknodWithContent(
		ctx,
		"large-file.bin",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		bytes.NewReader(content),
		0o644,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// verify file size
	fileHandle, err := fsHandle.Lookup(ctx, "large-file.bin")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fileHandle.Release()

	size, err := fileHandle.GetSize(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if size != uint64(len(content)) {
		t.Fatalf("expected size %d but got %d", len(content), size)
	}

	// read beginning and end to verify content
	headBuf := make([]byte, 100)
	n, err := fileHandle.ReadAt(ctx, 0, headBuf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(headBuf[:n], content[:100]) {
		t.Fatal("head content mismatch")
	}

	tailBuf := make([]byte, 100)
	tailOffset := int64(len(content) - 100)
	n, err = fileHandle.ReadAt(ctx, tailOffset, tailBuf)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bytes.Equal(tailBuf[:n], content[len(content)-100:]) {
		t.Fatal("tail content mismatch")
	}
}

// TestMknodWithContent_InSubdir tests creating a file in a subdirectory.
func TestMknodWithContent_InSubdir(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	watchWorldChanges := true
	fsHandle, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// create subdirectory
	err = fsHandle.MkdirAll(ctx, []string{"docs", "notes"}, 0o755, time.Now())
	if err != nil {
		t.Fatal(err.Error())
	}

	// navigate to the subdirectory
	subHandle, err := fsHandle.Lookup(ctx, "docs")
	if err != nil {
		t.Fatal(err.Error())
	}
	notesHandle, err := subHandle.Lookup(ctx, "notes")
	subHandle.Release()
	if err != nil {
		t.Fatal(err.Error())
	}
	defer notesHandle.Release()

	// create a file with content in the subdirectory
	content := []byte("notes content in subdir")
	err = notesHandle.MknodWithContent(
		ctx,
		"readme.txt",
		unixfs.NewFSCursorNodeType_File(),
		int64(len(content)),
		bytes.NewReader(content),
		0o644,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// verify via path lookup from root
	fileHandle, _, err := fsHandle.LookupPathPts(ctx, []string{"docs", "notes", "readme.txt"})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fileHandle.Release()

	size, err := fileHandle.GetSize(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if size != uint64(len(content)) {
		t.Fatalf("expected size %d but got %d", len(content), size)
	}
}

// TestFsBilly_WriteFile tests reading from a file immediately after writing it.
func TestFsBilly_WriteFile(t *testing.T) {
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	watchWorldChanges := true // TODO: test with both false/true
	fsHandle, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// create test fs (backed by a block graph + Hydra world)
	bfs := unixfs_billy.NewBillyFilesystem(ctx, fsHandle, "", time.Now())

	// create test script
	filename := "test.js"
	data := []byte("Hello world!\n")
	err = billy_util.WriteFile(bfs, filename, data, 0o755)
	if err != nil {
		t.Fatal(err.Error())
	}

	// read file size & check
	fi, err := bfs.Stat(filename)
	if err != nil {
		t.Fatal(err.Error())
	}
	if s := int(fi.Size()); s < len(data) {
		t.Fatalf("expected size %d but got %d", len(data), s)
	}
}
