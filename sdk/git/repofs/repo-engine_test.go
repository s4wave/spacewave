package repofs

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/go-git/go-git/v6/plumbing"
	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

func TestEngineCommit(t *testing.T) {
	// Create a repository object whose root can reveal committed changes.
	ctx := context.Background()
	ws, objState, oldRef := newEngineTestState(t, ctx, "repo/commit")
	defer world.ReleaseObjectState(objState)

	// Open a writable repository transaction and retain it through the commit.
	eng := NewEngine(ctx, ws, objState)
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Commit a main branch reference through the repository transaction.
	refName := plumbing.NewBranchReferenceName("main")
	refHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	if err := tx.SetReference(plumbing.NewHashReference(refName, refHash)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify that the commit replaced the repository object root.
	newRef, _, err := objState.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if newRef.GetRootRef().EqualsRef(oldRef.GetRootRef()) {
		t.Fatal("expected object root ref to change")
	}

	// Open a read transaction on the committed repository.
	readTx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	// Verify that the main branch reference persisted in the repository.
	readRef, err := readTx.Reference(refName)
	if err != nil {
		t.Fatal(err)
	}
	if readRef.Hash() != refHash {
		t.Fatal("expected committed ref to persist")
	}
}

func TestEngineDiscard(t *testing.T) {
	// Create a repository object whose root can reveal discarded changes.
	ctx := context.Background()
	ws, objState, oldRef := newEngineTestState(t, ctx, "repo/discard")
	defer world.ReleaseObjectState(objState)

	// Open a writable repository transaction for the discarded update.
	eng := NewEngine(ctx, ws, objState)
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Discard the main branch reference update without committing it.
	refName := plumbing.NewBranchReferenceName("main")
	refHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	if err := tx.SetReference(plumbing.NewHashReference(refName, refHash)); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	tx.Discard()

	// Verify that discarding the transaction preserved the repository root.
	newRef, _, err := objState.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !newRef.GetRootRef().EqualsRef(oldRef.GetRootRef()) {
		t.Fatal("expected discard to leave object root ref unchanged")
	}

	// Open a read transaction after discarding the repository update.
	readTx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	// Verify that the discarded main branch reference is absent.
	if _, err := readTx.Reference(refName); err != plumbing.ErrReferenceNotFound {
		t.Fatal("expected discarded ref change to be absent")
	}
}

func TestEngineChangeCb(t *testing.T) {
	// Create a repository object for observing committed changes.
	ctx := context.Background()
	ws, objState, _ := newEngineTestState(t, ctx, "repo/change-cb")
	defer world.ReleaseObjectState(objState)

	// Subscribe to repository changes through a buffered notification channel.
	eng := NewEngine(ctx, ws, objState)
	changeCh := make(chan struct{}, 1)
	rel := eng.AddDotGitChangeCb(func() {
		select {
		case changeCh <- struct{}{}:
		default:
		}
	})
	defer rel()

	// Commit a main branch reference to trigger a repository change.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	refName := plumbing.NewBranchReferenceName("main")
	refHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	if err := tx.SetReference(plumbing.NewHashReference(refName, refHash)); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		tx.Discard()
		t.Fatal(err)
	}
	tx.Discard()

	// Verify that the repository subscription receives the commit notification.
	select {
	case <-changeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for repo change callback")
	}
}

func TestOpenRepoFSCursorWriteCapability(t *testing.T) {
	// Create a repository object for testing cursor write capabilities.
	ctx := context.Background()
	ws, objectState, _ := newEngineTestState(t, ctx, "repo/cursor-capability")
	world.ReleaseObjectState(objectState)

	// Open a repository cursor with writes disabled.
	readOnlyCursor, err := OpenRepoFSCursor(ctx, ws, "repo/cursor-capability", false)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnlyCursor.Release()

	// Verify that the read-only repository cursor rejects writes.
	readOnlyOps, err := readOnlyCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := readOnlyOps.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected read-only cursor write to fail with ErrReadOnly, got %v", err)
	}

	// Open a repository cursor with writes enabled.
	writableCursor, err := OpenRepoFSCursor(ctx, ws, "repo/cursor-capability", true)
	if err != nil {
		t.Fatal(err)
	}
	defer writableCursor.Release()

	// Verify that the writable repository root still rejects file writes.
	writableOps, err := writableCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := writableOps.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrNotFile {
		t.Fatalf("expected writable root file write to fail with ErrNotFile, got %v", err)
	}

	// Open the writable repository reference directory.
	refsCursor, err := writableOps.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	defer refsCursor.Release()
	refsOps, err := refsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the writable repository branch directory.
	headsCursor, err := refsOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}
	defer headsCursor.Release()
	headsOps, err := headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Create the main branch reference through the writable cursor.
	refHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	refContent := []byte(refHash.String() + "\n")
	if err := headsOps.MknodWithContent(ctx, "main", unixfs.NewFSCursorNodeType_File(), int64(len(refContent)), bytes.NewReader(refContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}

	// Open a new read-only cursor on the committed repository.
	readCursor, err := OpenRepoFSCursor(ctx, ws, "repo/cursor-capability", false)
	if err != nil {
		t.Fatal(err)
	}
	defer readCursor.Release()
	readOps, err := readCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the committed repository reference directory.
	refsCursor, err = readOps.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	defer refsCursor.Release()
	refsOps, err = refsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the committed repository branch directory.
	headsCursor, err = refsOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}
	defer headsCursor.Release()
	headsOps, err = headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Open the committed main branch reference file.
	mainCursor, err := headsOps.Lookup(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer mainCursor.Release()
	mainOps, err := mainCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the committed branch file contains the written reference.
	buf := make([]byte, len(refContent))
	n, err := mainOps.ReadAt(ctx, 0, buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(refContent)) || !bytes.Equal(buf, refContent) {
		t.Fatalf("unexpected committed ref content %q", string(buf[:n]))
	}
}

func newEngineTestState(
	t *testing.T,
	ctx context.Context,
	objectKey string,
) (world.WorldState, world.ObjectState, *bucket.ObjectRef) {
	// Create the storage testbed for the repository World with helper attribution.
	t.Helper()
	log := logrus.New()
	le := logrus.NewEntry(log)
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Create the World testbed and release it when the test completes.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	// Register the Git operation controller on the World testbed bus.
	gitOpc := world.NewLookupOpController("test-git-repo-projection-engine", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Initialize the repository object through a World operation.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(objectKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Require the newly initialized repository object state.
	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected object state")
	}

	// Read the repository root reference for later change assertions.
	objRef, _, err := objState.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ws, objState, objRef
}
