package space_unixfs

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/bucket"
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

func TestFSCursorProjectsGitWorktreePaths(t *testing.T) {
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
	defer wtb.Release()

	unixfsOpc := world.NewLookupOpController("test-space-projection-git-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}
	gitOpc := world.NewLookupOpController("test-space-projection-git", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	if _, err := ws.CreateObject(ctx, "repo/demo", &bucket.ObjectRef{}); err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, ws, "repo/demo", git_world.GitRepoTypeID); err != nil {
		t.Fatal(err)
	}

	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: "repo/demo/workdir",
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	if err := git_world.CreateWorldObjectWorktree(
		ctx,
		le,
		ws,
		"repo/demo/worktree",
		"repo/demo",
		workdirRef,
		true,
		nil,
		sender,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	tx, err := wtb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	workdirCursor, _ := unixfs_world.NewFSCursorWithWriter(
		ctx,
		le,
		tx,
		"repo/demo/workdir",
		unixfs_world.FSType_FSType_FS_NODE,
		sender,
	)
	workdirHandle, err := unixfs.NewFSHandle(workdirCursor)
	if err != nil {
		workdirCursor.Release()
		t.Fatal(err)
	}
	defer workdirHandle.Release()

	now := time.Now()
	if err := workdirHandle.Mknod(ctx, true, []string{"README.md"}, unixfs.NewFSCursorNodeType_File(), 0o644, now); err != nil {
		t.Fatal(err)
	}
	fileHandle, _, err := workdirHandle.LookupPath(ctx, "README.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := fileHandle.WriteAt(ctx, 0, []byte("git worktree file"), now); err != nil {
		fileHandle.Release()
		t.Fatal(err)
	}
	fileHandle.Release()
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	rootCursor := NewFSCursor(le, world.NewEngineWorldState(wtb.Engine, false), 11, "space-git")
	rootHandle, err := unixfs.NewFSHandle(rootCursor)
	if err != nil {
		rootCursor.Release()
		t.Fatal(err)
	}
	defer rootHandle.Release()

	projectedFile, _, err := rootHandle.LookupPath(ctx, "u/11/so/space-git/-/repo/demo/worktree/-/README.md")
	if err != nil {
		t.Fatal(err)
	}
	defer projectedFile.Release()

	buf := make([]byte, 32)
	n, err := projectedFile.ReadAt(ctx, 0, buf)
	if err != nil && err != io.EOF {
		t.Fatal(err)
	}
	if got := string(buf[:n]); got != "git worktree file" {
		t.Fatalf("got %q, want %q", got, "git worktree file")
	}
}
