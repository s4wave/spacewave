package unixfs_world_testbed

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
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

// TestEngineWorldFilesystemSurvivesConcurrentWriteAndSync checks that a
// retained filesystem handle and another World writer cannot lose each other's
// committed values.
func TestEngineWorldFilesystemSurvivesConcurrentWriteAndSync(t *testing.T) {
	// Prepare the context and logger for concurrent World writes.
	ctx := context.Background()
	logger := logrus.New()
	le := logrus.NewEntry(logger)

	// Start the storage testbed and its World engine.
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err)
	}

	// Retain a writable filesystem handle after its initialization commit.
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fsHandle.Release)
	ws := world.NewEngineWorldState(wtb.Engine, true)
	initialSeqno, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Commit an unrelated object through another short transaction.
	const unrelatedKey = "test/concurrent-object"
	const unrelatedValue = "concurrent write survives"
	var createdObject world.ObjectState
	createdObject, _, err = world.CreateWorldObject(ctx, ws, unrelatedKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(block_mock.NewExample(unrelatedValue), true)
		return nil
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the unrelated World commit advances the sequence.
	concurrentSeqno, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if concurrentSeqno <= initialSeqno {
		t.Fatalf("expected concurrent write to advance seqno beyond %d, got %d", initialSeqno, concurrentSeqno)
	}

	// Write through the retained filesystem handle.
	const fileName = "retained-handle.txt"
	content := []byte("filesystem write survives")
	if err := fsHandle.Mknod(ctx, true, []string{fileName}, unixfs.NewFSCursorNodeType_File(), 0o644, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Open the retained filesystem file and write its content.
	fileHandle, err := fsHandle.Lookup(ctx, fileName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(fileHandle.Release)
	if err := fileHandle.WriteAt(ctx, 0, content, time.Now()); err != nil {
		t.Fatal(err)
	}
	fileHandle.Release()

	// Verify that the filesystem commit follows the unrelated commit.
	filesystemSeqno, err := ws.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if filesystemSeqno <= concurrentSeqno {
		t.Fatalf("expected filesystem write to advance seqno beyond %d, got %d", concurrentSeqno, filesystemSeqno)
	}

	// Release the retained handle before fencing durable storage.
	fsHandle.Release()
	if _, err := ws.Sync(ctx); err != nil {
		t.Fatal(err)
	}

	// Reopen from the engine and prove that neither writer lost the other.
	freshWS := world.NewEngineWorldState(wtb.Engine, false)
	unrelated, err := world.LookupObjectBody[*block_mock.Example](ctx, freshWS, unrelatedKey, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err)
	}
	if unrelated.GetMsg() != unrelatedValue {
		t.Fatalf("expected unrelated value %q, got %q", unrelatedValue, unrelated.GetMsg())
	}

	// Reopen the filesystem from the committed World root.
	freshFS, err := unixfs_world.BuildFSFromUnixfsRef(
		ctx,
		le,
		freshWS,
		"",
		&unixfs_world.UnixfsRef{ObjectKey: objKey},
		false,
		false,
		time.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(freshFS.Release)

	// Open the committed file for reading.
	freshFile, err := freshFS.Lookup(ctx, fileName)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(freshFile.Release)

	// Verify that the reopened file retains the filesystem write.
	read := make([]byte, len(content))
	n, err := freshFile.ReadAt(ctx, 0, read)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(content)) || !bytes.Equal(read, content) {
		t.Fatalf("expected file %q, got %q (%d bytes)", content, read, n)
	}
}

// TestFs runs the e2e tests.
func TestFs(t *testing.T) {
	// Prepare the context and logger for the filesystem checks.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the filesystem.
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine on the storage testbed.
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a filesystem that watches World changes.
	watchWorldChanges := true
	fsHandle, err := InitTestbed(wtb, objKey, watchWorldChanges)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Exercise the filesystem through the UnixFS contract checks.
	if err := unixfs_e2e.TestUnixFS(ctx, fsHandle); err != nil {
		t.Fatal(err.Error())
	}
}

// TestFs_SingleTxn runs the e2e tests with a single transaction.
func TestFs_SingleTxn(t *testing.T) {
	// Prepare the context and logger for transaction-backed filesystem checks.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the filesystem transaction.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for filesystem transactions.
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

		// Initialize the filesystem root in the write transaction.
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

	// Provide a filesystem handle backed by a fresh write transaction.
	buildFsh := func() (wtx world.Tx, fsh *unixfs.FSHandle, err error) {
		// Open the World write transaction for the filesystem handle.
		wtx, err = eng.NewTransaction(ctx, true)
		if err != nil {
			return nil, nil, err
		}

		// Wrap the writable filesystem cursor and release it if construction fails.
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
		// Open a temporary filesystem transaction and retain its cleanup.
		wtx, fsh, err := buildFsh()
		if err != nil {
			return err
		}
		defer wtx.Discard()
		defer fsh.Release()

		// Create the mydir directory in the temporary filesystem.
		if err := fsh.Mknod(ctx, false, []string{"mydir"}, unixfs.NewFSCursorNodeType_Dir(), 0o644, time.Now()); err != nil {
			return err
		}

		// Create the nested test directory in the temporary filesystem.
		if err := fsh.MkdirAll(ctx, []string{"test", "dir"}, 0o700, time.Now()); err != nil {
			return err
		}

		// Create the nested file path in the temporary filesystem.
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
		// Open the filesystem transaction for the full contract checks.
		wtx, fsh, err := buildFsh()
		if err != nil {
			return err
		}
		defer wtx.Discard()
		defer fsh.Release()

		// Exercise the UnixFS contract within the write transaction.
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
	// Prepare the context and logger for atomic file creation.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for atomic file creation.
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the filesystem.
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a filesystem that watches the atomic file commit.
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
	// Prepare the context and logger for chunked file creation.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the chunked file.
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the chunked filesystem.
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a filesystem that watches the chunked file commit.
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

	// Verify the stored size of the chunked file.
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

	// Verify the content at the end of the chunked file.
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
	// Prepare the context and logger for subdirectory file creation.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the nested filesystem.
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the nested filesystem.
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a filesystem that watches nested file commits.
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

	// Verify the nested file size through its root-relative handle.
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
	// Prepare the context and logger for the Billy filesystem adapter.
	ctx := context.Background()
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(logger)

	// Start the storage testbed for the Billy filesystem.
	tb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Start the World engine for the Billy filesystem.
	wtb, err := world_testbed.NewTestbed(tb)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open a filesystem that watches writes through the Billy adapter.
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
