package s4wave_git_test

import (
	"context"
	"testing"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	space_world "github.com/s4wave/spacewave/core/space/world"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

func setupGitWizardWorld(t *testing.T) (context.Context, *world_testbed.Testbed, world.WorldState) {
	t.Helper()

	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	gitOpc := world.NewLookupOpController("test-alpha-git-wizard-ops", tb.EngineID, git_world.LookupGitOp)
	if _, err := tb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		t.Fatal(err)
	}

	return ctx, tb, world.NewEngineWorldState(tb.Engine, true)
}

func TestCreateGitRepoWizardOpCreatesTypedRepo(t *testing.T) {
	ctx, tb, ws := setupGitWizardWorld(t)
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

func worldContentsHasObject(contents *space_world.WorldContents, objectKey, typeID string) bool {
	for _, obj := range contents.GetObjects() {
		if obj.GetObjectKey() == objectKey && obj.GetObjectType() == typeID {
			return true
		}
	}
	return false
}
