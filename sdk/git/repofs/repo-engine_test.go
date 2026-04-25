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
	ctx := context.Background()
	ws, objState, oldRef := newEngineTestState(t, ctx, "repo/commit")

	eng := NewEngine(ctx, ws, objState)
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	refName := plumbing.NewBranchReferenceName("main")
	refHash := plumbing.NewHash("1111111111111111111111111111111111111111")
	if err := tx.SetReference(plumbing.NewHashReference(refName, refHash)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	newRef, _, err := objState.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if newRef.GetRootRef().EqualsRef(oldRef.GetRootRef()) {
		t.Fatal("expected object root ref to change")
	}

	readTx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	readRef, err := readTx.Reference(refName)
	if err != nil {
		t.Fatal(err)
	}
	if readRef.Hash() != refHash {
		t.Fatal("expected committed ref to persist")
	}
}

func TestEngineDiscard(t *testing.T) {
	ctx := context.Background()
	ws, objState, oldRef := newEngineTestState(t, ctx, "repo/discard")

	eng := NewEngine(ctx, ws, objState)
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
	tx.Discard()

	newRef, _, err := objState.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !newRef.GetRootRef().EqualsRef(oldRef.GetRootRef()) {
		t.Fatal("expected discard to leave object root ref unchanged")
	}

	readTx, err := eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	if _, err := readTx.Reference(refName); err != plumbing.ErrReferenceNotFound {
		t.Fatal("expected discarded ref change to be absent")
	}
}

func TestEngineChangeCb(t *testing.T) {
	ctx := context.Background()
	ws, objState, _ := newEngineTestState(t, ctx, "repo/change-cb")

	eng := NewEngine(ctx, ws, objState)
	changeCh := make(chan struct{}, 1)
	rel := eng.AddDotGitChangeCb(func() {
		select {
		case changeCh <- struct{}{}:
		default:
		}
	})
	defer rel()

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

	select {
	case <-changeCh:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for repo change callback")
	}
}

func TestOpenRepoFSCursorWriteCapability(t *testing.T) {
	ctx := context.Background()
	ws, _, _ := newEngineTestState(t, ctx, "repo/cursor-capability")

	readOnlyCursor, err := OpenRepoFSCursor(ctx, ws, "repo/cursor-capability", false)
	if err != nil {
		t.Fatal(err)
	}
	defer readOnlyCursor.Release()

	readOnlyOps, err := readOnlyCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := readOnlyOps.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrReadOnly {
		t.Fatalf("expected read-only cursor write to fail with ErrReadOnly, got %v", err)
	}

	writableCursor, err := OpenRepoFSCursor(ctx, ws, "repo/cursor-capability", true)
	if err != nil {
		t.Fatal(err)
	}
	defer writableCursor.Release()

	writableOps, err := writableCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := writableOps.WriteAt(ctx, 0, []byte("x"), time.Time{}); err != unixfs_errors.ErrNotFile {
		t.Fatalf("expected writable root file write to fail with ErrNotFile, got %v", err)
	}
	refsCursor, err := writableOps.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	defer refsCursor.Release()
	refsOps, err := refsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	headsCursor, err := refsOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}
	defer headsCursor.Release()
	headsOps, err := headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	refHash := plumbing.NewHash("2222222222222222222222222222222222222222")
	refContent := []byte(refHash.String() + "\n")
	if err := headsOps.MknodWithContent(ctx, "main", unixfs.NewFSCursorNodeType_File(), int64(len(refContent)), bytes.NewReader(refContent), 0o644, time.Time{}); err != nil {
		t.Fatal(err)
	}

	readCursor, err := OpenRepoFSCursor(ctx, ws, "repo/cursor-capability", false)
	if err != nil {
		t.Fatal(err)
	}
	defer readCursor.Release()
	readOps, err := readCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	refsCursor, err = readOps.Lookup(ctx, "refs")
	if err != nil {
		t.Fatal(err)
	}
	defer refsCursor.Release()
	refsOps, err = refsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	headsCursor, err = refsOps.Lookup(ctx, "heads")
	if err != nil {
		t.Fatal(err)
	}
	defer headsCursor.Release()
	headsOps, err = headsCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mainCursor, err := headsOps.Lookup(ctx, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer mainCursor.Release()
	mainOps, err := mainCursor.GetCursorOps(ctx)
	if err != nil {
		t.Fatal(err)
	}
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
	t.Helper()

	log := logrus.New()
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

	gitOpc := world.NewLookupOpController("test-git-repo-projection-engine", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(objectKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	objState, found, err := ws.GetObject(ctx, objectKey)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("expected object state")
	}

	objRef, _, err := objState.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return ws, objState, objRef
}
