package resource_git

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
	"github.com/sirupsen/logrus"
)

type testWatchStatusStream struct {
	srpc.Stream
	ctx  context.Context
	msgs chan *s4wave_git.WatchStatusResponse
}

func newTestWatchStatusStream(ctx context.Context) *testWatchStatusStream {
	return &testWatchStatusStream{
		ctx:  ctx,
		msgs: make(chan *s4wave_git.WatchStatusResponse, 4),
	}
}

func (m *testWatchStatusStream) Context() context.Context {
	return m.ctx
}

func (m *testWatchStatusStream) Send(resp *s4wave_git.WatchStatusResponse) error {
	select {
	case m.msgs <- resp:
		return nil
	case <-m.ctx.Done():
		return m.ctx.Err()
	}
}

func (m *testWatchStatusStream) SendAndClose(resp *s4wave_git.WatchStatusResponse) error {
	return m.Send(resp)
}

func (m *testWatchStatusStream) MsgRecv(_ srpc.Message) error {
	return nil
}

func (m *testWatchStatusStream) MsgSend(_ srpc.Message) error {
	return nil
}

func (m *testWatchStatusStream) CloseSend() error {
	return nil
}

func (m *testWatchStatusStream) Close() error {
	return nil
}

func TestGitWorktreeResourceStatusStageUnstageUsesWorkdir(t *testing.T) {
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

	unixfsOpc := world.NewLookupOpController("test-git-worktree-resource-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}
	gitOpc := world.NewLookupOpController("test-git-worktree-resource-git", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/worktree-resource"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	workdir := memfs.New()
	var firstHash string
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, repoKey, true, nil, workdir, nil, func(repo *git.Repository) error {
		hash, err := commitResourceReadme(repo, workdir, "# Demo\n", "initial commit")
		if err != nil {
			return err
		}
		firstHash = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	workdirRef := &unixfs_world.UnixfsRef{
		ObjectKey: workdirKey,
		FsType:    unixfs_world.FSType_FSType_FS_NODE,
	}
	if err := git_world.CreateWorldObjectWorktree(
		ctx,
		le,
		ws,
		worktreeKey,
		repoKey,
		workdirRef,
		true,
		&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("master"), Force: true},
		sender,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	err = git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, repoKey, worktreeKey, time.Now(), true, sender, func(repo *git.Repository, workdir billy.Filesystem) error {
		f, err := workdir.Create("README.md")
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte("# Demo changed\n")); err != nil {
			_ = f.Close()
			return err
		}
		return f.Close()
	})
	if err != nil {
		t.Fatal(err)
	}

	resource := NewGitWorktreeResource(ws, wtb.Engine, worktreeKey, &WorktreeSnapshot{
		RepoObjectKey:    repoKey,
		WorkdirObjectKey: workdirKey,
		WorkdirRef:       workdirRef,
		CheckedOutRef:    "master",
		HeadCommitHash:   firstHash,
		HasWorkdir:       true,
	})

	status, err := watchResourceStatus(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}
	entry := findResourceStatus(t, status, "README.md")
	if entry.GetStagingStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED || entry.GetWorktreeStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_MODIFIED {
		t.Fatalf("unexpected modified status staging=%s worktree=%s", entry.GetStagingStatus().String(), entry.GetWorktreeStatus().String())
	}

	if _, err := resource.StageFiles(ctx, &s4wave_git.StageFilesRequest{Paths: []string{"README.md"}}); err != nil {
		t.Fatal(err)
	}
	status, err = watchResourceStatus(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}
	entry = findResourceStatus(t, status, "README.md")
	if entry.GetStagingStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_MODIFIED || entry.GetWorktreeStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED {
		t.Fatalf("unexpected staged status staging=%s worktree=%s", entry.GetStagingStatus().String(), entry.GetWorktreeStatus().String())
	}

	if _, err := resource.UnstageFiles(ctx, &s4wave_git.UnstageFilesRequest{Paths: []string{"README.md"}}); err != nil {
		t.Fatal(err)
	}
	status, err = watchResourceStatus(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}
	entry = findResourceStatus(t, status, "README.md")
	if entry.GetStagingStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED || entry.GetWorktreeStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_MODIFIED {
		t.Fatalf("unexpected unstaged status staging=%s worktree=%s", entry.GetStagingStatus().String(), entry.GetWorktreeStatus().String())
	}
}

func commitResourceReadme(repo *git.Repository, workdir billy.Filesystem, content, message string) (string, error) {
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}
	f, err := workdir.Create("README.md")
	if err != nil {
		return "", err
	}
	if _, err := f.Write([]byte(content)); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if _, err := wt.Add("README.md"); err != nil {
		return "", err
	}
	hash, err := wt.Commit(message, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test",
			Email: "test@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

func watchResourceStatus(ctx context.Context, resource *GitWorktreeResource) (*s4wave_git.WatchStatusResponse, error) {
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newTestWatchStatusStream(watchCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- resource.WatchStatus(&s4wave_git.WatchStatusRequest{}, stream)
	}()
	var resp *s4wave_git.WatchStatusResponse
	select {
	case resp = <-stream.msgs:
	case <-watchCtx.Done():
		return nil, watchCtx.Err()
	}
	cancel()
	select {
	case err := <-errCh:
		if err != nil && err != context.Canceled {
			return nil, err
		}
	case <-time.After(time.Second):
		return nil, context.DeadlineExceeded
	}
	return resp, nil
}

func findResourceStatus(t *testing.T, status *s4wave_git.WatchStatusResponse, path string) *s4wave_git.StatusEntry {
	t.Helper()
	for _, entry := range status.GetEntries() {
		if entry.GetFilePath() == path {
			return entry
		}
	}
	t.Fatalf("missing status entry for %s", path)
	return nil
}
