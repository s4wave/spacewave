package s4wave_git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	_ "github.com/go-git/go-git/v6/plumbing/transport/file"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	space_world "github.com/s4wave/spacewave/core/space/world"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

func setupGitWorld(t *testing.T) (context.Context, *world_testbed.Testbed, world.WorldState) {
	t.Helper()

	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	gitOpc := world.NewLookupOpController("test-alpha-git-ops", tb.EngineID, git_world.LookupGitOp)
	if _, err := tb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	return ctx, tb, world.NewEngineWorldState(tb.Engine, true)
}

func TestCreateGitRepoWizardOpCreatesTypedRepo(t *testing.T) {
	ctx, tb, ws := setupGitWorld(t)
	objectKey := "repo/wizard-init"

	op := &s4wave_git.CreateGitRepoWizardOp{
		ObjectKey: objectKey,
		Timestamp: timestamppb.Now(),
	}
	_, _, err := ws.ApplyWorldOp(ctx, op, tb.Volume.GetPeerID())
	if err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	typeID, err := world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("GetObjectType: %v", err)
	}
	if typeID != git_world.GitRepoTypeID {
		t.Fatalf("expected type %q, got %q", git_world.GitRepoTypeID, typeID)
	}

	contents, err := space_world.BuildWorldContents(ctx, ws)
	if err != nil {
		t.Fatalf("BuildWorldContents: %v", err)
	}
	if !worldContentsHasObject(contents, objectKey, git_world.GitRepoTypeID) {
		t.Fatalf("world contents missing typed repo %q: %#v", objectKey, contents.GetObjects())
	}
}

func TestCloneGitRepoToRefPublishesTypedRepo(t *testing.T) {
	ctx, _, ws := setupGitWorld(t)
	sourcePath := createSourceRepo(t)
	objectKey := "repo/imported"

	repoRef, err := s4wave_git.CloneGitRepoToRef(ctx, ws, &git_block.CloneOpts{
		Url: sourcePath,
	}, nil, nil)
	if err != nil {
		t.Fatalf("CloneGitRepoToRef: %v", err)
	}

	initOp := git_world.NewGitInitOp(objectKey, repoRef, true, nil, timestamppb.Now())
	_, _, err = ws.ApplyWorldOp(ctx, initOp, "")
	if err != nil {
		t.Fatalf("ApplyWorldOp(publish): %v", err)
	}

	typeID, err := world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("GetObjectType: %v", err)
	}
	if typeID != git_world.GitRepoTypeID {
		t.Fatalf("expected type %q, got %q", git_world.GitRepoTypeID, typeID)
	}
}

func createSourceRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainInit(dir, false)
	if err != nil {
		t.Fatalf("PlainInit: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Demo\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("Worktree: %v", err)
	}
	if _, err := wt.Add("README.md"); err != nil {
		t.Fatalf("Add: %v", err)
	}
	_, err = wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Tester",
			Email: "tester@example.com",
			When:  time.Now(),
		},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	return dir
}

func worldContentsHasObject(contents *space_world.WorldContents, objectKey, typeID string) bool {
	for _, obj := range contents.GetObjects() {
		if obj.GetObjectKey() == objectKey && obj.GetObjectType() == typeID {
			return true
		}
	}
	return false
}
