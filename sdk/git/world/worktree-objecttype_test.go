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

func TestGitWorktreeFactoryAllowsUnbornHead(t *testing.T) {
	ctx := context.Background()
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
	defer wtb.Release()

	ws := world.NewEngineWorldState(wtb.Engine, true)
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
	if err := git_world.CreateWorldObjectWorktree(
		ctx,
		le,
		ws,
		worktreeKey,
		repoKey,
		workdirRef,
		true,
		nil,
		sender,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

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
