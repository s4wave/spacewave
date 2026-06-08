package forge_lib_git_commit

import (
	"context"
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing"
	"github.com/go-git/go-git/v6/plumbing/object"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	git_world "github.com/s4wave/spacewave/db/git/world"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
	resource_git "github.com/s4wave/spacewave/sdk/git/resource"
	"github.com/sirupsen/logrus"
)

type captureHandle struct {
	peerID     peer.ID
	ts         *timestamp.Timestamp
	accessFunc world.AccessWorldStateFunc
	outputs    forge_value.ValueSlice
}

func (h *captureHandle) GetExecutionUniqueId() string {
	return "test-git-commit"
}

func (h *captureHandle) GetPeerId() peer.ID {
	return h.peerID
}

func (h *captureHandle) GetTimestamp() *timestamp.Timestamp {
	return h.ts
}

func (h *captureHandle) AccessStorage(ctx context.Context, ref *bucket.ObjectRef, cb func(*bucket_lookup.Cursor) error) error {
	return h.accessFunc(ctx, ref, cb)
}

func (h *captureHandle) SetOutputs(ctx context.Context, outps forge_value.ValueSlice, clearOld bool) error {
	if clearOld {
		h.outputs = nil
	}
	h.outputs = append(h.outputs, outps.Clone()...)
	return nil
}

func (h *captureHandle) WriteLog(ctx context.Context, level, message string) error {
	return nil
}

func TestGitCommitControllerCommitsStagedWorktreeAndOutputsResult(t *testing.T) {
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

	unixfsOpc := world.NewLookupOpController("test-git-commit-unixfs", wtb.EngineID, unixfs_world.LookupFsOp)
	if _, err := wtb.Bus.AddController(ctx, unixfsOpc, nil); err != nil {
		t.Fatal(err)
	}
	gitOpc := world.NewLookupOpController("test-git-commit-git", wtb.EngineID, git_world.LookupGitOp)
	if _, err := wtb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender := wtb.Volume.GetPeerID()
	repoKey := "repo/forge-commit"
	worktreeKey := repoKey + "/worktree"
	workdirKey := repoKey + "/workdir"
	if _, _, err := ws.ApplyWorldOp(ctx, git_world.NewGitInitOp(repoKey, nil, true, nil, nil), sender); err != nil {
		t.Fatal(err)
	}

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

	handle := &captureHandle{
		peerID:     sender,
		ts:         timestamp.Now(),
		accessFunc: ws.AccessWorldState,
	}
	inputs := forge_target.InputMap{
		inputNameWorld: forge_target.NewInputValueWorld(wtb.Engine, ws),
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

	err = git_world.AccessWorldObjectRepoWithWorktree(ctx, le, ws, repoKey, worktreeKey, time.Now(), false, "", func(repo *git.Repository, workdir billy.Filesystem) error {
		wt, err := repo.Worktree()
		if err != nil {
			return err
		}
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

func commitTestReadme(repo *git.Repository, workdir billy.Filesystem, content, message string) (string, error) {
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
