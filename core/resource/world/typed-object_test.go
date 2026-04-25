package resource_world_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v6"
	"github.com/go-git/go-git/v6/plumbing/object"
	_ "github.com/go-git/go-git/v6/plumbing/transport/file"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	core_git "github.com/s4wave/spacewave/core/git"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/bucket"
	git_block "github.com/s4wave/spacewave/db/git/block"
	git_world "github.com/s4wave/spacewave/db/git/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_git "github.com/s4wave/spacewave/sdk/git"
	s4wave_git_world "github.com/s4wave/spacewave/sdk/git/world"
	s4wave_layout_world "github.com/s4wave/spacewave/sdk/layout/world"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
)

// TestTypedObjectResource tests the TypedObjectResourceService.
func TestTypedObjectResource(t *testing.T) {
	ctx := context.Background()

	tb, tbCleanup := setupWorldTestbed(ctx, t)
	defer tbCleanup()

	t.Run("AccessTypedObjectNotFound", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClientWithObjectTypes(ctx, t, tb)
		defer cleanup()

		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer tx.Release()

		// Try to access a non-existent object
		srpcClient, err := tx.GetResourceRef().GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}
		typedSvcClient := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
		_, err = typedSvcClient.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
			ObjectKey: "nonexistent-object",
		})
		if err == nil {
			t.Fatal("expected error for nonexistent object")
		}

		t.Logf("Correctly returned error for nonexistent object: %v", err)
	})

	t.Run("AccessTypedObjectNoType", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClientWithObjectTypes(ctx, t, tb)
		defer cleanup()

		// Create an object without a type using the SDK
		sdkTx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}

		objectKey := "test-untyped-object-" + t.Name()
		obj, err := sdkTx.CreateObject(ctx, objectKey, &bucket.ObjectRef{})
		if err != nil {
			sdkTx.Release()
			t.Fatalf("CreateObject failed: %v", err)
		}
		_ = obj
		err = sdkTx.Commit(ctx)
		if err != nil {
			sdkTx.Release()
			t.Fatalf("Commit failed: %v", err)
		}
		sdkTx.Release()

		// Create a new read transaction to try to access the typed object
		readTx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer readTx.Release()

		// Verify the object was created
		obj2, found, err := readTx.GetObject(ctx, objectKey)
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}
		if !found {
			t.Fatal("expected object to exist")
		}
		t.Logf("Created object with key: %s", obj2.GetKey())

		// Now try to access it as a typed object - should fail because it has no type
		srpcClient, err := readTx.GetResourceRef().GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}
		typedSvcClient := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
		_, err = typedSvcClient.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
			ObjectKey: objectKey,
		})
		if err == nil {
			t.Fatal("expected error for object without type")
		}

		t.Logf("Correctly returned error for object without type: %v", err)

		// Cleanup: release the object reference
		if objRef, ok := obj2.(interface{ Release() }); ok {
			objRef.Release()
		}
		_ = resClient // referenced for cleanup func
	})

	t.Run("AccessTypedObjectLayout", func(t *testing.T) {
		_, engine, cleanup := setupWorldResourceClientWithObjectTypes(ctx, t, tb)
		defer cleanup()

		// Create an ObjectLayout using the InitObjectLayoutOp
		objectKey := "object-layout/test-layout"

		sdkTx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}

		// Create and serialize the operation
		op := space_world_ops.NewInitObjectLayoutOp(objectKey, time.Now())
		opData, err := op.MarshalBlock()
		if err != nil {
			sdkTx.Release()
			t.Fatalf("MarshalBlock failed: %v", err)
		}

		// Apply the operation
		_, _, err = sdkTx.ApplyWorldOp(ctx, space_world_ops.InitObjectLayoutOpId, opData, "")
		if err != nil {
			sdkTx.Release()
			t.Fatalf("ApplyWorldOp failed: %v", err)
		}

		// Commit the transaction
		err = sdkTx.Commit(ctx)
		if err != nil {
			sdkTx.Release()
			t.Fatalf("Commit failed: %v", err)
		}
		sdkTx.Release()

		// Create a new read transaction to access the typed object
		readTx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer readTx.Release()

		// Verify the object was created
		obj, found, err := readTx.GetObject(ctx, objectKey)
		if err != nil {
			t.Fatalf("GetObject failed: %v", err)
		}
		if !found {
			t.Fatal("expected object to exist")
		}
		t.Logf("Created object with key: %s", obj.GetKey())

		// Access the typed object
		srpcClient, err := readTx.GetResourceRef().GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}
		typedSvcClient := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
		resp, err := typedSvcClient.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
			ObjectKey: objectKey,
		})
		if err != nil {
			t.Fatalf("AccessTypedObject failed: %v", err)
		}

		// Verify the response
		if resp.TypeId != s4wave_layout_world.ObjectLayoutTypeID {
			t.Fatalf("expected type %q, got %q", s4wave_layout_world.ObjectLayoutTypeID, resp.TypeId)
		}
		if resp.ResourceId == 0 {
			t.Fatal("expected non-zero resource ID")
		}

		t.Logf("Successfully accessed typed object with type=%s resourceId=%d", resp.TypeId, resp.ResourceId)
	})

	t.Run("AccessTypedObjectGitRepo", func(t *testing.T) {
		resClient, engine, cleanup := setupWorldResourceClientWithObjectTypes(ctx, t, tb)
		defer cleanup()

		gitOpc := world.NewLookupOpController("test-typed-object-git", tb.EngineID, git_world.LookupGitOp)
		gitOpRelease, err := tb.Bus.AddController(ctx, gitOpc, nil)
		if err != nil {
			t.Fatalf("AddController(git op): %v", err)
		}
		defer gitOpRelease()

		objectKey := "repo/typed-object"
		repoRef, err := core_git.CloneGitRepoToRef(ctx, tb.Engine, &git_block.CloneOpts{
			Url: createTypedObjectSourceRepo(t),
		}, nil, nil)
		if err != nil {
			t.Fatalf("CloneGitRepoToRef: %v", err)
		}
		sdkTx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		op := git_world.NewGitInitOp(objectKey, repoRef, true, nil, nil)
		opData, err := op.MarshalBlock()
		if err != nil {
			sdkTx.Release()
			t.Fatalf("MarshalBlock failed: %v", err)
		}
		_, _, err = sdkTx.ApplyWorldOp(ctx, git_world.GitInitOpId, opData, "")
		if err != nil {
			sdkTx.Release()
			t.Fatalf("ApplyWorldOp failed: %v", err)
		}
		if err := sdkTx.Commit(ctx); err != nil {
			sdkTx.Release()
			t.Fatalf("Commit failed: %v", err)
		}
		sdkTx.Release()

		readTx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction failed: %v", err)
		}
		defer readTx.Release()

		srpcClient, err := readTx.GetResourceRef().GetClient()
		if err != nil {
			t.Fatalf("GetClient failed: %v", err)
		}
		typedSvcClient := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
		resp, err := typedSvcClient.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
			ObjectKey: objectKey,
		})
		if err != nil {
			t.Fatalf("AccessTypedObject failed: %v", err)
		}
		if resp.GetTypeId() != s4wave_git_world.GitRepoTypeID {
			t.Fatalf("expected type %q, got %q", s4wave_git_world.GitRepoTypeID, resp.GetTypeId())
		}

		gitRef := resClient.CreateResourceReference(resp.GetResourceId())
		defer gitRef.Release()
		gitClient, err := gitRef.GetClient()
		if err != nil {
			t.Fatalf("GetClient(git): %v", err)
		}
		gitSvc := s4wave_git.NewSRPCGitRepoResourceServiceClient(gitClient)
		info, err := gitSvc.GetRepoInfo(ctx, &s4wave_git.GetRepoInfoRequest{})
		if err != nil {
			t.Fatalf("GetRepoInfo: %v", err)
		}
		if info.GetIsEmpty() {
			t.Fatal("expected imported repo with commits")
		}
		if _, err := gitSvc.ListRefs(ctx, &s4wave_git.ListRefsRequest{}); err != nil {
			t.Fatalf("ListRefs: %v", err)
		}
		if _, err := gitSvc.Log(ctx, &s4wave_git.LogRequest{}); err != nil {
			t.Fatalf("Log: %v", err)
		}
		treeResp, err := gitSvc.GetTreeResource(ctx, &s4wave_git.GetTreeResourceRequest{})
		if err != nil {
			t.Fatalf("GetTreeResource: %v", err)
		}
		treeRef := resClient.CreateResourceReference(treeResp.GetResourceId())
		treeRef.Release()
	})
}

func createTypedObjectSourceRepo(t *testing.T) string {
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

// setupWorldResourceClientWithObjectTypes sets up resource client with ObjectType controller.
func setupWorldResourceClientWithObjectTypes(ctx context.Context, t *testing.T, tb *world_testbed.Testbed) (*resource_client.Client, *s4wave_world.Engine, func()) {
	// Register ObjectType controller with known types on the testbed's bus
	objectTypes := map[string]objecttype.ObjectType{
		s4wave_layout_world.ObjectLayoutTypeID: s4wave_layout_world.ObjectLayoutType,
		s4wave_git_world.GitRepoTypeID:         s4wave_git_world.GitRepoType,
	}
	lookupFunc := func(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
		return objectTypes[typeID], nil
	}
	objectTypeCtrl := objecttype_controller.NewController(lookupFunc)
	objectTypeCtrlRelease, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		t.Fatalf("Failed to add ObjectType controller: %v", err)
	}

	resClient, clientCleanup := resource_testbed.SetupResourceClient(ctx, t, tb)

	rootRef := resClient.AccessRootResource()

	srpcClient, err := rootRef.GetClient()
	if err != nil {
		objectTypeCtrlRelease()
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createWorldResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		objectTypeCtrlRelease()
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	engineRef := resClient.CreateResourceReference(createWorldResp.ResourceId)
	engine, err := s4wave_world.NewEngine(resClient, engineRef)
	if err != nil {
		objectTypeCtrlRelease()
		rootRef.Release()
		clientCleanup()
		t.Fatal(err.Error())
	}

	cleanup := func() {
		engine.Release()
		rootRef.Release()
		clientCleanup()
		objectTypeCtrlRelease()
	}

	return resClient, engine, cleanup
}
