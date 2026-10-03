package resource_git

import (
	"context"
	"strings"
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
	// Prepare the context and logger for the worktree testbed.
	ctx := context.Background()
	log := logrus.New()
	le := logrus.NewEntry(log)

	// Start the block-storage testbed for the saved repository.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}

	// Start the World testbed that stores the repository and workdir.
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Register the filesystem and Git operations used by the worktree.
	unixfsOpc := world.NewLookupOpController("test-git-worktree-resource-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}
	gitOpc := world.NewLookupOpController("test-git-worktree-resource-git", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	// Create the repository object and keys for its worktree and workdir.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/worktree-resource"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Create the first README commit in the saved repository.
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

	// Link the mutable workdir and check out the initial commit.
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

	// Modify the checked-out README through the saved workdir.
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

	// Expose the saved worktree through its resource snapshot.
	resource := NewGitWorktreeResource(ws, wtb.Engine, worktreeKey, &WorktreeSnapshot{
		RepoObjectKey:    repoKey,
		WorkdirObjectKey: workdirKey,
		WorkdirRef:       workdirRef,
		CheckedOutRef:    "master",
		HeadCommitHash:   firstHash,
		HasWorkdir:       true,
	})

	// Verify the README change appears only in workdir status.
	status, err := watchResourceStatus(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the initial README change is confined to the workdir.
	entry := findResourceStatus(t, status, "README.md")
	if entry.GetStagingStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED || entry.GetWorktreeStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_MODIFIED {
		t.Fatalf("unexpected modified status staging=%s worktree=%s", entry.GetStagingStatus().String(), entry.GetWorktreeStatus().String())
	}

	// Stage the README and verify its change moves into the index.
	if _, err := resource.StageFiles(ctx, &s4wave_git.StageFilesRequest{Paths: []string{"README.md"}}); err != nil {
		t.Fatal(err)
	}
	status, err = watchResourceStatus(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the staged README has no remaining workdir change.
	entry = findResourceStatus(t, status, "README.md")
	if entry.GetStagingStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_MODIFIED || entry.GetWorktreeStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED {
		t.Fatalf("unexpected staged status staging=%s worktree=%s", entry.GetStagingStatus().String(), entry.GetWorktreeStatus().String())
	}

	// Unstage the README and verify its workdir change remains.
	if _, err := resource.UnstageFiles(ctx, &s4wave_git.UnstageFilesRequest{Paths: []string{"README.md"}}); err != nil {
		t.Fatal(err)
	}
	status, err = watchResourceStatus(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}

	// Verify unstaging preserves the README workdir change.
	entry = findResourceStatus(t, status, "README.md")
	if entry.GetStagingStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED || entry.GetWorktreeStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_MODIFIED {
		t.Fatalf("unexpected unstaged status staging=%s worktree=%s", entry.GetStagingStatus().String(), entry.GetWorktreeStatus().String())
	}

	// Verify a commit rejects the unstaged README without changing status.
	if _, err := resource.CommitFiles(ctx, &s4wave_git.CommitFilesRequest{
		Paths:           []string{"README.md"},
		Message:         "should reject unstaged path",
		AuthorName:      "Test",
		AuthorEmail:     "test@example.com",
		AuthorTimestamp: time.Now().Unix(),
	}); err == nil || !strings.Contains(err.Error(), "path is not staged: README.md") {
		t.Fatalf("expected unstaged commit rejection, got %v", err)
	}
	status, err = watchResourceStatus(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the rejected commit preserves the README status.
	entry = findResourceStatus(t, status, "README.md")
	if entry.GetStagingStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_UNMODIFIED || entry.GetWorktreeStatus() != s4wave_git.FileStatusCode_FILE_STATUS_CODE_MODIFIED {
		t.Fatalf("failed commit should leave status inspectable staging=%s worktree=%s", entry.GetStagingStatus().String(), entry.GetWorktreeStatus().String())
	}

	// Stage the README before introducing an extra staged path.
	if _, err := resource.StageFiles(ctx, &s4wave_git.StageFilesRequest{Paths: []string{"README.md"}}); err != nil {
		t.Fatal(err)
	}

	// Add an extra staged file outside the requested commit selection.
	err = git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, repoKey, worktreeKey, time.Now(), true, sender, func(repo *git.Repository, workdir billy.Filesystem) error {
		// Create the extra workdir file for commit-selection validation.
		f, err := workdir.Create("extra.txt")
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte("extra\n")); err != nil {
			_ = f.Close()
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}

		// Stage the extra file in the repository index.
		wt, err := repo.Worktree()
		if err != nil {
			return err
		}
		_, err = wt.Add("extra.txt")
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the commit rejects staged paths outside its selection.
	if _, err := resource.CommitFiles(ctx, &s4wave_git.CommitFilesRequest{
		Paths:           []string{"README.md"},
		Message:         "should reject extra staged path",
		AuthorName:      "Test",
		AuthorEmail:     "test@example.com",
		AuthorTimestamp: time.Now().Unix(),
	}); err == nil || !strings.Contains(err.Error(), "unexpected staged path: extra.txt") {
		t.Fatalf("expected extra staged path rejection, got %v", err)
	}

	// Remove the extra file from the index and workdir.
	if _, err := resource.UnstageFiles(ctx, &s4wave_git.UnstageFilesRequest{Paths: []string{"extra.txt"}}); err != nil {
		t.Fatal(err)
	}
	err = git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, repoKey, worktreeKey, time.Now(), true, sender, func(repo *git.Repository, workdir billy.Filesystem) error {
		return workdir.Remove("extra.txt")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Commit the selected README with the default author timestamp.
	commitResp, err := resource.CommitFiles(ctx, &s4wave_git.CommitFilesRequest{
		Paths:       []string{"README.md"},
		Message:     "update readme",
		AuthorName:  "Test",
		AuthorEmail: "test@example.com",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the commit response describes the new commit and its base.
	if commitResp.GetCommitHash() == "" || commitResp.GetCommitHash() == firstHash {
		t.Fatalf("commit hash: got %q first %q", commitResp.GetCommitHash(), firstHash)
	}
	if commitResp.GetBaseCommitHash() != firstHash ||
		commitResp.GetBranchRef() != "master" ||
		len(commitResp.GetAffectedPaths()) != 1 ||
		commitResp.GetAffectedPaths()[0] != "README.md" {
		t.Fatalf("commit response: %+v", commitResp)
	}

	// Verify the saved commit records a nonzero default author timestamp.
	err = git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, repoKey, worktreeKey, time.Now(), false, "", func(repo *git.Repository, workdir billy.Filesystem) error {
		commit, err := repo.CommitObject(plumbing.NewHash(commitResp.GetCommitHash()))
		if err != nil {
			return err
		}
		if commit.Author.When.IsZero() || commit.Author.When.Equal(time.Unix(0, 0)) {
			t.Fatalf("expected omitted author timestamp to default away from Unix epoch, got %s", commit.Author.When)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the resource retains its initial metadata snapshot after commit.
	info, err := resource.GetWorktreeInfo(ctx, &s4wave_git.GetWorktreeInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if info.GetHeadCommitHash() != firstHash {
		t.Fatalf("snapshot head should remain initial until resource is reloaded: %+v", info)
	}

	// Verify the committed worktree has no remaining status entries.
	status, err = watchResourceStatus(ctx, resource)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.GetEntries()) != 0 {
		t.Fatalf("commit should clean worktree status: %+v", status.GetEntries())
	}
}

func commitResourceReadme(repo *git.Repository, workdir billy.Filesystem, content, message string) (string, error) {
	// Open the repository worktree for the README commit.
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	// Write and close the README in the workdir.
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

	// Stage and commit the README with the test author.
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
	// Start a cancellable worktree status stream.
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newTestWatchStatusStream(watchCtx)
	errCh := make(chan error, 1)
	go func() {
		errCh <- resource.WatchStatus(&s4wave_git.WatchStatusRequest{}, stream)
	}()

	// Receive the first worktree snapshot from the stream.
	var resp *s4wave_git.WatchStatusResponse
	select {
	case resp = <-stream.msgs:
	case <-watchCtx.Done():
		return nil, watchCtx.Err()
	}

	// Cancel the watch and verify the streaming routine exits.
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
