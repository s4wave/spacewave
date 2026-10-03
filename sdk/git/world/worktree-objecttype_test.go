package s4wave_git_world

import (
	"context"
	"testing"
	"time"

	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// newTestWorld returns an engine World state over a fresh testbed.
func newTestWorld(t *testing.T) (context.Context, *logrus.Entry, *world_testbed.Testbed, world.WorldState) {
	// Build the volume testbed.
	t.Helper()
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Build the World testbed on the volume.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)
	return ctx, le, wtb, world.NewEngineWorldState(wtb.Engine, true)
}

func TestGitRepoFactoryReadsNewRepo(t *testing.T) {
	// Open a fresh engine World.
	ctx, le, wtb, ws := newTestWorld(t)

	// Create an empty repository without a worktree.
	repoKey := "repo/empty-repo-factory"
	sender := wtb.Volume.GetPeerID()
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Open the repository resource, which reads through engine storage.
	mux, cleanup, err := GitRepoFactory(ctx, le, nil, wtb.Engine, ws, repoKey)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if mux == nil {
		t.Fatal("expected Git repo resource mux")
	}
}

func TestGitWorktreeFactoryAllowsUnbornHead(t *testing.T) {
	// Open a fresh engine World.
	ctx, le, wtb, ws := newTestWorld(t)

	// Create an empty repository and a worktree attached to it.
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/empty-worktree-factory"
	worktreeKey := repoKey + "/worktree"
	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: repoKey + "/workdir",
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}
	if err := git_world.CreateWorldObjectWorktree(ctx, le, ws, worktreeKey, repoKey, workdirRef, true, nil, sender, time.Now()); err != nil {
		t.Fatal(err)
	}

	// Open the worktree resource while HEAD is still unborn.
	mux, cleanup, err := GitWorktreeFactory(ctx, le, nil, wtb.Engine, ws, worktreeKey)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup != nil {
		defer cleanup()
	}
	if mux == nil {
		t.Fatal("expected Git worktree resource mux")
	}
}
