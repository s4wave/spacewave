package s4wave_git_test

import (
	"context"
	"testing"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// encryptedSDKWorld drives the SDK cursor path against an encrypted world:
// the world engine lives behind SRPC and its bucket enforces a block
// transform, like a daemon serving an encrypted space.
type encryptedSDKWorld struct {
	ctx        context.Context
	resClient  *resource_client.Client
	resourceID uint32
	release    func()
}

func (w *encryptedSDKWorld) openEngine() (world.WorldState, func(), error) {
	engineRef := w.resClient.CreateResourceReference(w.resourceID)
	engine, err := sdk_world_engine.NewSDKEngine(w.resClient, engineRef)
	if err != nil {
		engineRef.Release()
		return nil, nil, err
	}
	return world.NewEngineWorldState(engine, true), engine.Release, nil
}

func setupEncryptedSDKWorld(t *testing.T) *encryptedSDKWorld {
	t.Helper()

	ctx := t.Context()
	tb, resClient, tbCleanup := resource_testbed.SetupTestbedWithClient(ctx, t)

	gitOpc := world.NewLookupOpController("test-git-encrypted-srpc", tb.EngineID, git_world.LookupGitOp)
	if _, err := tb.Bus.AddController(ctx, gitOpc, nil); err != nil {
		tbCleanup()
		t.Fatal(err.Error())
	}

	rootRef := resClient.AccessRootResource()
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		rootRef.Release()
		tbCleanup()
		t.Fatal(err.Error())
	}
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		rootRef.Release()
		tbCleanup()
		t.Fatal(err.Error())
	}

	return &encryptedSDKWorld{
		ctx:        ctx,
		resClient:  resClient,
		resourceID: createResp.GetResourceId(),
		release: func() {
			rootRef.Release()
			tbCleanup()
		},
	}
}

func TestCloneGitRepoToRefSurvivesEncryptedCursorReopen(t *testing.T) {
	w := setupEncryptedSDKWorld(t)
	defer w.release()

	ws, closeEngine, err := w.openEngine()
	if err != nil {
		t.Fatalf("NewSDKEngine: %v", err)
	}
	defer closeEngine()

	objectKey := "repo/imported-encrypted"
	repoRef, err := s4wave_git.CloneGitRepoToRef(w.ctx, ws, &git_block.CloneOpts{
		Url: createSourceRepo(t),
	}, nil, nil)
	if err != nil {
		t.Fatalf("CloneGitRepoToRef: %v", err)
	}

	initOp := git_world.NewGitInitOp(objectKey, repoRef, true, nil, timestamppb.Now())
	if _, _, err := ws.ApplyWorldOp(w.ctx, initOp, ""); err != nil {
		t.Fatalf("ApplyWorldOp(publish): %v", err)
	}
	// Keep the first engine open: releasing its resource reference stops the
	// daemon-side world engine, so reopen here means a fresh SDK cursor that
	// re-reads the transform config and re-decodes the encrypted repo blocks.
	// The deferred closeEngine runs at test end.
	reopenedWS, closeReopened, err := w.openEngine()
	if err != nil {
		t.Fatalf("NewSDKEngine(reopen): %v", err)
	}
	defer closeReopened()

	typeID, err := world_types.GetObjectType(w.ctx, reopenedWS, objectKey)
	if err != nil {
		t.Fatalf("GetObjectType after reopen: %v", err)
	}
	if typeID != git_world.GitRepoTypeID {
		t.Fatalf("expected type %q after reopen, got %q", git_world.GitRepoTypeID, typeID)
	}
}
