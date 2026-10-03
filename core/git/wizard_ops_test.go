package s4wave_git_test

import (
	"context"
	"testing"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

func setupGitWizardWorld(t *testing.T) (context.Context, *world_testbed.Testbed, world.WorldState) {
	// Attribute wizard World setup failures to the calling test.
	t.Helper()

	// Start a World testbed and release its resources after the test.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Attach the Git operation controller used by the repository wizard.
	gitOpc := world.NewLookupOpController("test-alpha-git-wizard-ops", tb.EngineID, git_world.LookupGitOp)
	if _, err := tb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	return ctx, tb, world.NewEngineWorldState(tb.Engine, true)
}

func TestCreateGitRepoWizardOpCreatesTypedRepo(t *testing.T) {
	// Build a World with the git operation controller.
	ctx, tb, ws := setupGitWizardWorld(t)
	objectKey := "repo/wizard-init"

	// Apply the wizard operation.
	op := &s4wave_git.CreateGitRepoWizardOp{
		ObjectKey: objectKey,
		Timestamp: timestamppb.Now(),
	}
	_, _, err := ws.ApplyWorldOp(ctx, op, tb.Volume.GetPeerID())
	if err != nil {
		t.Fatalf("ApplyWorldOp: %v", err)
	}

	// Check the repo type.
	typeID, err := world_types.GetObjectType(ctx, ws, objectKey)
	if err != nil {
		t.Fatalf("GetObjectType: %v", err)
	}
	if typeID != git_world.GitRepoTypeID {
		t.Fatalf("expected type %q, got %q", git_world.GitRepoTypeID, typeID)
	}

	// Check the worktree type.
	worktreeType, err := world_types.GetObjectType(ctx, ws, objectKey+"/worktree")
	if err != nil {
		t.Fatalf("GetObjectType: %v", err)
	}
	if worktreeType != git_world.GitWorktreeTypeID {
		t.Fatalf("expected worktree type %q, got %q", git_world.GitWorktreeTypeID, worktreeType)
	}
}
