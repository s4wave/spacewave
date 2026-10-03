package forge_lib_git_commit

import (
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
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
	forge_target "github.com/s4wave/spacewave/forge/target"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
	resource_git "github.com/s4wave/spacewave/sdk/git/resource"
	"github.com/sirupsen/logrus"
)

// TestGitCommitControllerCommitsStagedWorktreeAndOutputsResult commits staged files and retains the typed result.
func TestGitCommitControllerCommitsStagedWorktreeAndOutputsResult(t *testing.T) {
	// Open an isolated World backed by in-memory Git storage.
	ctx := t.Context()
	log := logrus.New()
	le := logrus.NewEntry(log)

	// Retain the block store and World for the commit controller.
	btb, err := hydra_testbed.NewTestbed(ctx, le, hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(btb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	// Register the World operations required by the commit controller.
	unixfsOpc := world.NewLookupOpController("test-git-commit-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	unixfsRef, err := wtb.Bus.AddController(ctx, unixfsOpc, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unixfsRef)
	gitOpc := world.NewLookupOpController("test-git-commit-git", wtb.EngineID, git_world.LookupGitOp)
	gitRef, err := wtb.Bus.AddController(ctx, gitOpc, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(gitRef)

	// Initialize the repository and its owned worktree keys.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/forge-commit"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

	// Seed the repository with a first commit to compare against the controller result.
	workdir := memfs.New()
	var firstHash string
	_, _, err = git_world.AccessWorldObjectRepo(ctx, ws, repoKey, true, nil, workdir, nil, func(repo *git.Repository) error {
		hash, err := commitTestReadme(repo, workdir, "# Demo\n", "initial commit")
		if err != nil {
			return err
		}
		firstHash = hash
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Check out the initial commit into the World worktree.
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

	// Change the worktree file without staging it.
	err = git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, repoKey, worktreeKey, time.Now(), true, sender, func(repo *git.Repository, workdir billy.Filesystem) error {
		// Write the changed README in the borrowed worktree.
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

	// Stage the commit controller's storage access.
	stage, err := ws.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stage.Release()

	// Capture outputs through a handle that writes through the stage.
	handle := &captureHandle{
		peerID:     sender,
		ts:         timestamp.Now(),
		accessFunc: stage.AccessWorldState,
	}
	inputs := forge_target.InputMap{
		inputNameWorld: forge_target.NewInputValueWorld(wtb.EngineID, wtb.Engine, ws),
	}
	conf := &Config{
		WorktreeObjectKey: worktreeKey,
		RepoObjectKey:     repoKey,
		CommitRequest: &s4wave_git.CommitFilesRequest{
			Paths:           []string{"README.md"},
			Message:         "update readme",
			AuthorName:      "Test",
			AuthorEmail:     "test@example.com",
			AuthorTimestamp: time.Now().Unix(),
		},
	}
	controller := NewController(le, nil, conf)
	if err := controller.InitForgeExecController(ctx, inputs, handle); err != nil {
		t.Fatal(err)
	}
	if err := controller.Execute(ctx); err == nil {
		t.Fatal("expected unstaged commit to fail")
	}

	// Stage the changed file and execute the commit through the controller.
	resource := resource_git.NewGitWorktreeResource(ws, wtb.Engine, worktreeKey, &resource_git.WorktreeSnapshot{RepoObjectKey: repoKey})
	if _, err := resource.StageFiles(ctx, &s4wave_git.StageFilesRequest{Paths: []string{"README.md"}}); err != nil {
		t.Fatal(err)
	}
	if err := controller.Execute(ctx); err != nil {
		t.Fatal(err)
	}
	if len(handle.outputs) != 1 {
		t.Fatalf("expected one output, got %d", len(handle.outputs))
	}

	// Decode the controller's retained commit response.
	out := handle.outputs[0]
	if out.GetName() != outputNameCommit || out.IsEmpty() {
		t.Fatalf("unexpected output: %+v", out)
	}
	data, err := forge_target.LoadBlobValueToBytes(ctx, handle, out)
	if err != nil {
		t.Fatal(err)
	}
	var resp s4wave_git.CommitFilesResponse
	if err := resp.UnmarshalVT(data); err != nil {
		t.Fatal(err)
	}
	if resp.GetCommitHash() == "" || resp.GetCommitHash() == firstHash {
		t.Fatalf("commit hash: got %q first %q", resp.GetCommitHash(), firstHash)
	}
	if resp.GetBaseCommitHash() != firstHash ||
		resp.GetBranchRef() != "master" ||
		len(resp.GetAffectedPaths()) != 1 ||
		resp.GetAffectedPaths()[0] != "README.md" {
		t.Fatalf("commit response: %+v", &resp)
	}

	// Inspect the worktree after the committed update.
	err = git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, repoKey, worktreeKey, time.Now(), false, "", func(repo *git.Repository, workdir billy.Filesystem) error {
		// Access the repository worktree to inspect its status.
		wt, err := repo.Worktree()
		if err != nil {
			return err
		}

		// Require the committed worktree to have no remaining staged changes.
		status, err := wt.Status()
		if err != nil {
			return err
		}
		if !status.IsClean() {
			t.Fatalf("commit should clean worktree status: %+v", status)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// commitTestReadme writes and commits a README in the in-memory repository.
func commitTestReadme(repo *git.Repository, workdir billy.Filesystem, content, message string) (string, error) {
	// Access the repository worktree for the initial commit.
	wt, err := repo.Worktree()
	if err != nil {
		return "", err
	}

	// Write the README content before staging it.
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
