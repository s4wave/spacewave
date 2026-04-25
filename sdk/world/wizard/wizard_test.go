package s4wave_wizard_test

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	forge_job_ops "github.com/s4wave/spacewave/core/forge/job"
	forge_task_ops "github.com/s4wave/spacewave/core/forge/task"
	s4wave_git "github.com/s4wave/spacewave/core/git"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	git_block "github.com/s4wave/spacewave/db/git/block"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	bifcrypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
	"github.com/s4wave/spacewave/sdk/world/objecttype"
	objecttype_controller "github.com/s4wave/spacewave/sdk/world/objecttype/controller"
	s4wave_wizard "github.com/s4wave/spacewave/sdk/world/wizard"
)

// setupWizardWorldEngine creates a world engine with the wizard object type
// controller registered.
func setupWizardWorldEngine(ctx context.Context, t *testing.T) (*resource_client.Client, *s4wave_world.Engine, func()) {
	t.Helper()

	tb, resClient, tbCleanup := resource_testbed.SetupTestbedWithClient(ctx, t)

	lookupFunc := func(ctx context.Context, typeID string) (objecttype.ObjectType, error) {
		return s4wave_wizard.LookupWizardObjectType(ctx, typeID)
	}
	objectTypeCtrl := objecttype_controller.NewController(lookupFunc)
	objectTypeCtrlRelease, err := tb.Bus.AddController(ctx, objectTypeCtrl, nil)
	if err != nil {
		tbCleanup()
		t.Fatalf("add ObjectType controller: %v", err)
	}

	rootRef := resClient.AccessRootResource()
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		objectTypeCtrlRelease()
		rootRef.Release()
		tbCleanup()
		t.Fatalf("get root client: %v", err)
	}

	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{})
	if err != nil {
		objectTypeCtrlRelease()
		rootRef.Release()
		tbCleanup()
		t.Fatalf("create world: %v", err)
	}

	engineRef := resClient.CreateResourceReference(createResp.ResourceId)
	engine, err := s4wave_world.NewEngine(resClient, engineRef)
	if err != nil {
		engineRef.Release()
		objectTypeCtrlRelease()
		rootRef.Release()
		tbCleanup()
		t.Fatalf("create engine: %v", err)
	}

	cleanup := func() {
		engine.Release()
		objectTypeCtrlRelease()
		rootRef.Release()
		_, _, ref, err := bus.ExecOneOffTyped[objecttype.ObjectType](
			ctx,
			tb.Bus,
			objecttype.NewLookupObjectType("wizard/cleanup"),
			bus.ReturnWhenIdle(),
			nil,
		)
		if err == nil && ref != nil {
			ref.Release()
		}
		tbCleanup()
	}

	return resClient, engine, cleanup
}

// accessWizardResource opens a wizard object through the typed-object service.
func accessWizardResource(
	ctx context.Context,
	t *testing.T,
	resClient *resource_client.Client,
	engine *s4wave_world.Engine,
	objectKey string,
) (*s4wave_world.Tx, resource_client.ResourceRef, s4wave_wizard.SRPCWizardResourceServiceClient) {
	t.Helper()

	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(read): %v", err)
	}

	srpcClient, err := readTx.GetResourceRef().GetClient()
	if err != nil {
		readTx.Release()
		t.Fatalf("GetClient: %v", err)
	}

	typedSvc := s4wave_world.NewSRPCTypedObjectResourceServiceClient(srpcClient)
	resp, err := typedSvc.AccessTypedObject(ctx, &s4wave_world.AccessTypedObjectRequest{
		ObjectKey: objectKey,
	})
	if err != nil {
		readTx.Release()
		t.Fatalf("AccessTypedObject: %v", err)
	}
	if resp.GetTypeId() != "wizard/test" {
		readTx.Release()
		t.Fatalf("expected type wizard/test, got %q", resp.GetTypeId())
	}

	wizardRef := resClient.CreateResourceReference(resp.GetResourceId())
	wizardClient, err := wizardRef.GetClient()
	if err != nil {
		wizardRef.Release()
		readTx.Release()
		t.Fatalf("GetClient(wizard): %v", err)
	}

	return readTx, wizardRef, s4wave_wizard.NewSRPCWizardResourceServiceClient(wizardClient)
}

// recvWizardState receives one wizard-state snapshot from a watch stream.
func recvWizardState(
	ctx context.Context,
	t *testing.T,
	wizardSvc s4wave_wizard.SRPCWizardResourceServiceClient,
) *s4wave_wizard.WizardState {
	t.Helper()

	watchCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	stream, err := wizardSvc.WatchWizardState(watchCtx, &s4wave_wizard.WatchWizardStateRequest{})
	if err != nil {
		t.Fatalf("WatchWizardState: %v", err)
	}

	msg, err := stream.Recv()
	if err != nil {
		t.Fatalf("WatchWizardState.Recv: %v", err)
	}
	if msg.GetState() == nil {
		t.Fatal("expected wizard state")
	}

	return msg.GetState()
}

// TestWizardResourcePersistsState verifies the persistent wizard flow for a
// test type: create a wizard object, update it through WizardResourceService,
// then re-open it through a fresh typed-object resource and verify the updated
// state is read back from the world.
func TestWizardResourcePersistsState(t *testing.T) {
	ctx := context.Background()
	resClient, engine, cleanup := setupWizardWorldEngine(ctx, t)
	defer cleanup()

	objectKey := "wizard/test-canvas"
	createOp := s4wave_wizard.NewCreateWizardObjectOp(
		objectKey,
		"wizard/test",
		"canvas",
		"canvas/",
		"Draft Canvas",
		time.Now(),
	)
	createOpData, err := createOp.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT(create op): %v", err)
	}

	writeTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(write): %v", err)
	}
	_, _, err = writeTx.ApplyWorldOp(ctx, s4wave_wizard.CreateWizardObjectOpId, createOpData, "")
	if err != nil {
		writeTx.Release()
		t.Fatalf("ApplyWorldOp: %v", err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Release()
		t.Fatalf("Commit(create): %v", err)
	}
	writeTx.Release()

	readTx, wizardRef, wizardSvc := accessWizardResource(ctx, t, resClient, engine, objectKey)
	initialState := recvWizardState(ctx, t, wizardSvc)
	if initialState.GetStep() != 0 {
		t.Fatalf("expected initial step 0, got %d", initialState.GetStep())
	}
	if initialState.GetTargetTypeId() != "canvas" {
		t.Fatalf("expected initial target type canvas, got %q", initialState.GetTargetTypeId())
	}
	if initialState.GetTargetKeyPrefix() != "canvas/" {
		t.Fatalf("expected initial key prefix canvas/, got %q", initialState.GetTargetKeyPrefix())
	}
	if initialState.GetName() != "Draft Canvas" {
		t.Fatalf("expected initial name Draft Canvas, got %q", initialState.GetName())
	}

	updateResp, err := wizardSvc.UpdateWizardState(ctx, &s4wave_wizard.UpdateWizardStateRequest{
		Step: 1,
		Name: "Configured Canvas",
	})
	wizardRef.Release()
	readTx.Release()
	if err != nil {
		t.Fatalf("UpdateWizardState: %v", err)
	}
	if updateResp.GetState().GetStep() != 1 {
		t.Fatalf("expected updated step 1, got %d", updateResp.GetState().GetStep())
	}
	if updateResp.GetState().GetName() != "Configured Canvas" {
		t.Fatalf("expected updated name Configured Canvas, got %q", updateResp.GetState().GetName())
	}

	verifyTx, verifyRef, verifySvc := accessWizardResource(ctx, t, resClient, engine, objectKey)
	defer verifyRef.Release()
	defer verifyTx.Release()

	persistedState := recvWizardState(ctx, t, verifySvc)
	if persistedState.GetStep() != 1 {
		t.Fatalf("expected persisted step 1, got %d", persistedState.GetStep())
	}
	if persistedState.GetName() != "Configured Canvas" {
		t.Fatalf("expected persisted name Configured Canvas, got %q", persistedState.GetName())
	}
	if persistedState.GetTargetTypeId() != "canvas" {
		t.Fatalf("expected persisted target type canvas, got %q", persistedState.GetTargetTypeId())
	}
	if persistedState.GetTargetKeyPrefix() != "canvas/" {
		t.Fatalf("expected persisted key prefix canvas/, got %q", persistedState.GetTargetKeyPrefix())
	}
}

// TestClusterWizardFinalize verifies the cluster wizard finalize flow: create a
// wizard object for forge/cluster, then apply ClusterCreateOp with empty peerId
// and a non-empty sender. The Go handler defaults peerId to sender. The cluster
// object is created and the wizard object is deleted.
func TestClusterWizardFinalize(t *testing.T) {
	ctx := context.Background()
	_, engine, cleanup := setupWizardWorldEngine(ctx, t)
	defer cleanup()

	// Generate a test peer ID for the sender.
	priv, _, err := bifcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	senderPeerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}

	// Step 1: Create wizard object for forge/cluster.
	wizardKey := "wizard/forge/cluster/test-1"
	createWizardOp := s4wave_wizard.NewCreateWizardObjectOp(
		wizardKey,
		"wizard/forge/cluster",
		"forge/cluster",
		"forge/cluster/",
		"test-cluster",
		time.Now(),
	)
	wizardOpData, err := createWizardOp.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT(wizard op): %v", err)
	}

	writeTx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(create wizard): %v", err)
	}
	_, _, err = writeTx.ApplyWorldOp(ctx, s4wave_wizard.CreateWizardObjectOpId, wizardOpData, "")
	if err != nil {
		writeTx.Release()
		t.Fatalf("ApplyWorldOp(create wizard): %v", err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		writeTx.Release()
		t.Fatalf("Commit(create wizard): %v", err)
	}
	writeTx.Release()

	// Step 2: Simulate finalize - apply ClusterCreateOp with empty peerId.
	// The Go handler defaults peerId to the sender.
	clusterKey := "forge/cluster/test-cluster-abc"
	clusterOp := &forge_cluster.ClusterCreateOp{
		ClusterKey: clusterKey,
		Name:       "test-cluster",
		PeerId:     "", // empty: defaults to sender
	}
	clusterOpData, err := clusterOp.MarshalVT()
	if err != nil {
		t.Fatalf("MarshalVT(cluster op): %v", err)
	}

	writeTx2, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("NewTransaction(create cluster): %v", err)
	}
	_, _, err = writeTx2.ApplyWorldOp(ctx, forge_cluster.ClusterCreateOpId, clusterOpData, senderPeerID.String())
	if err != nil {
		writeTx2.Release()
		t.Fatalf("ApplyWorldOp(create cluster): %v", err)
	}

	// Step 3: Delete the wizard object (simulating finalize cleanup).
	_, err = writeTx2.DeleteObject(ctx, wizardKey)
	if err != nil {
		writeTx2.Release()
		t.Fatalf("DeleteObject(wizard): %v", err)
	}
	if err := writeTx2.Commit(ctx); err != nil {
		writeTx2.Release()
		t.Fatalf("Commit(finalize): %v", err)
	}
	writeTx2.Release()

	// Step 4: Verify cluster exists and wizard is gone.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(verify): %v", err)
	}
	defer readTx.Release()

	clusterObj, found, err := readTx.GetObject(ctx, clusterKey)
	if err != nil {
		t.Fatalf("GetObject(cluster): %v", err)
	}
	if !found || clusterObj == nil {
		t.Fatal("cluster object not found after finalize")
	}

	_, wizardFound, err := readTx.GetObject(ctx, wizardKey)
	if err != nil {
		t.Fatalf("GetObject(wizard): %v", err)
	}
	if wizardFound {
		t.Fatal("wizard object should be deleted after finalize")
	}
}

// TestForgeWizardChain verifies the full forge wizard creation chain:
// cluster -> job -> task. Each entity is created through the wizard finalize
// pattern (create wizard, apply target op, delete wizard). Verifies all three
// objects exist, all wizard objects are deleted, and graph edges
// (cluster-to-job, job-to-task) are correct.
func TestForgeWizardChain(t *testing.T) {
	ctx := context.Background()
	_, engine, cleanup := setupWizardWorldEngine(ctx, t)
	defer cleanup()

	// Generate a test peer ID for the sender.
	priv, _, err := bifcrypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateEd25519Key: %v", err)
	}
	senderPeerID, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("IDFromPrivateKey: %v", err)
	}
	sender := senderPeerID.String()

	clusterKey := "forge/cluster/chain-test"
	jobKey := "forge/job/chain-test-job"
	taskKey := "forge/task/chain-test-task"
	clusterWizardKey := "wizard/forge/cluster/chain-1"
	jobWizardKey := "wizard/forge/job/chain-1"
	taskWizardKey := "wizard/forge/task/chain-1"

	// Phase 1: Create cluster via wizard finalize.
	{
		wizardOp := s4wave_wizard.NewCreateWizardObjectOp(
			clusterWizardKey, "wizard/forge/cluster",
			"forge/cluster", "forge/cluster/", "chain-cluster", time.Now(),
		)
		wizardData, err := wizardOp.MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT(cluster wizard): %v", err)
		}

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction(cluster wizard create): %v", err)
		}
		_, _, err = tx.ApplyWorldOp(ctx, s4wave_wizard.CreateWizardObjectOpId, wizardData, "")
		if err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp(cluster wizard): %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit(cluster wizard): %v", err)
		}
		tx.Release()

		// Finalize: create cluster + delete wizard.
		clusterOp := &forge_cluster.ClusterCreateOp{
			ClusterKey: clusterKey,
			Name:       "chain-cluster",
		}
		clusterData, err := clusterOp.MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT(cluster op): %v", err)
		}
		tx, err = engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction(cluster finalize): %v", err)
		}
		_, _, err = tx.ApplyWorldOp(ctx, forge_cluster.ClusterCreateOpId, clusterData, sender)
		if err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp(cluster create): %v", err)
		}
		_, err = tx.DeleteObject(ctx, clusterWizardKey)
		if err != nil {
			tx.Release()
			t.Fatalf("DeleteObject(cluster wizard): %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit(cluster finalize): %v", err)
		}
		tx.Release()
	}

	// Phase 2: Create job via wizard finalize (linked to cluster).
	{
		wizardOp := s4wave_wizard.NewCreateWizardObjectOp(
			jobWizardKey, "wizard/forge/job",
			"forge/job", "forge/job/", "chain-job", time.Now(),
		)
		wizardData, err := wizardOp.MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT(job wizard): %v", err)
		}

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction(job wizard create): %v", err)
		}
		_, _, err = tx.ApplyWorldOp(ctx, s4wave_wizard.CreateWizardObjectOpId, wizardData, "")
		if err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp(job wizard): %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit(job wizard): %v", err)
		}
		tx.Release()

		// Finalize: create job with one task def, assigned to cluster, delete wizard.
		jobOp := &forge_job_ops.ForgeJobCreateOp{
			JobKey:     jobKey,
			ClusterKey: clusterKey,
			TaskDefs:   []*forge_job_ops.ForgeJobTaskDef{{Name: "build"}},
			Timestamp:  timestamppb.Now(),
		}
		jobData, err := jobOp.MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT(job op): %v", err)
		}
		tx, err = engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction(job finalize): %v", err)
		}
		_, _, err = tx.ApplyWorldOp(ctx, forge_job_ops.ForgeJobCreateOpId, jobData, sender)
		if err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp(job create): %v", err)
		}
		_, err = tx.DeleteObject(ctx, jobWizardKey)
		if err != nil {
			tx.Release()
			t.Fatalf("DeleteObject(job wizard): %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit(job finalize): %v", err)
		}
		tx.Release()
	}

	// Phase 3: Create task via wizard finalize (linked to job).
	{
		wizardOp := s4wave_wizard.NewCreateWizardObjectOp(
			taskWizardKey, "wizard/forge/task",
			"forge/task", "forge/task/", "chain-task", time.Now(),
		)
		wizardData, err := wizardOp.MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT(task wizard): %v", err)
		}

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction(task wizard create): %v", err)
		}
		_, _, err = tx.ApplyWorldOp(ctx, s4wave_wizard.CreateWizardObjectOpId, wizardData, "")
		if err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp(task wizard): %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit(task wizard): %v", err)
		}
		tx.Release()

		// Finalize: create task linked to job, delete wizard.
		taskOp := &forge_task_ops.ForgeTaskCreateOp{
			TaskKey:   taskKey,
			Name:      "chain-task",
			JobKey:    jobKey,
			Timestamp: timestamppb.Now(),
		}
		taskData, err := taskOp.MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT(task op): %v", err)
		}
		tx, err = engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction(task finalize): %v", err)
		}
		_, _, err = tx.ApplyWorldOp(ctx, forge_task_ops.ForgeTaskCreateOpId, taskData, sender)
		if err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp(task create): %v", err)
		}
		_, err = tx.DeleteObject(ctx, taskWizardKey)
		if err != nil {
			tx.Release()
			t.Fatalf("DeleteObject(task wizard): %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit(task finalize): %v", err)
		}
		tx.Release()
	}

	// Verify: all three objects exist, all wizard objects deleted, graph edges correct.
	readTx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction(verify): %v", err)
	}
	defer readTx.Release()

	// Objects exist.
	for _, key := range []string{clusterKey, jobKey, taskKey} {
		_, found, err := readTx.GetObject(ctx, key)
		if err != nil {
			t.Fatalf("GetObject(%s): %v", key, err)
		}
		if !found {
			t.Fatalf("object %s not found", key)
		}
	}

	// Wizard objects deleted.
	for _, key := range []string{clusterWizardKey, jobWizardKey, taskWizardKey} {
		_, found, err := readTx.GetObject(ctx, key)
		if err != nil {
			t.Fatalf("GetObject(%s): %v", key, err)
		}
		if found {
			t.Fatalf("wizard object %s should be deleted", key)
		}
	}

	// Graph edge: cluster -> job (predicate forge/cluster-job).
	clusterJobQuads, err := readTx.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(clusterKey, forge_cluster.PredClusterToJob.String(), jobKey, ""),
		0,
	)
	if err != nil {
		t.Fatalf("LookupGraphQuads(cluster->job): %v", err)
	}
	if len(clusterJobQuads) == 0 {
		t.Fatal("missing cluster-to-job graph edge")
	}

	// Graph edge: job -> task (predicate forge/job-task).
	jobTaskQuads, err := readTx.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(jobKey, forge_job.PredJobToTask.String(), taskKey, ""),
		0,
	)
	if err != nil {
		t.Fatalf("LookupGraphQuads(job->task): %v", err)
	}
	if len(jobTaskQuads) == 0 {
		t.Fatal("missing job-to-task graph edge")
	}

}

// TestGitRepoWizardOp verifies the CreateGitRepoWizardOp validation and
// dispatch logic, and the wizard create/delete lifecycle for git/repo.
func TestGitRepoWizardOp(t *testing.T) {
	ctx := context.Background()

	t.Run("validate", func(t *testing.T) {
		// Valid new repo op.
		op := &s4wave_git.CreateGitRepoWizardOp{
			ObjectKey: "git/repo/test",
			Name:      "test",
			Timestamp: timestamppb.Now(),
		}
		if err := op.Validate(); err != nil {
			t.Fatalf("Validate(new repo) should pass: %v", err)
		}

		// Valid clone op.
		cloneOp := &s4wave_git.CreateGitRepoWizardOp{
			ObjectKey: "git/repo/cloned",
			Name:      "cloned",
			Clone:     true,
			CloneOpts: &git_block.CloneOpts{
				Url: "https://github.com/example/test.git",
				Ref: "main",
			},
			Timestamp: timestamppb.Now(),
		}
		if err := cloneOp.Validate(); err != nil {
			t.Fatalf("Validate(clone) should pass: %v", err)
		}

		// Missing object key.
		badOp := &s4wave_git.CreateGitRepoWizardOp{Name: "test"}
		if err := badOp.Validate(); err == nil {
			t.Fatal("Validate should fail with empty object_key")
		}

		// Clone without clone_opts.
		badClone := &s4wave_git.CreateGitRepoWizardOp{
			ObjectKey: "git/repo/test",
			Clone:     true,
		}
		if err := badClone.Validate(); err == nil {
			t.Fatal("Validate should fail: clone=true without clone_opts")
		}
	})

	t.Run("lookup", func(t *testing.T) {
		op, err := s4wave_git.LookupCreateGitRepoWizardOp(ctx, s4wave_git.CreateGitRepoWizardOpId)
		if err != nil {
			t.Fatalf("LookupCreateGitRepoWizardOp: %v", err)
		}
		if op == nil {
			t.Fatal("LookupCreateGitRepoWizardOp returned nil for matching ID")
		}
		if op.GetOperationTypeId() != s4wave_git.CreateGitRepoWizardOpId {
			t.Fatalf("expected type ID %q, got %q", s4wave_git.CreateGitRepoWizardOpId, op.GetOperationTypeId())
		}

		// Non-matching ID returns nil.
		op, err = s4wave_git.LookupCreateGitRepoWizardOp(ctx, "other/op")
		if err != nil {
			t.Fatalf("LookupCreateGitRepoWizardOp(other): %v", err)
		}
		if op != nil {
			t.Fatal("expected nil for non-matching ID")
		}
	})

	t.Run("marshal-roundtrip", func(t *testing.T) {
		op := &s4wave_git.CreateGitRepoWizardOp{
			ObjectKey: "git/repo/rt-test",
			Name:      "rt-test",
			Clone:     true,
			CloneOpts: &git_block.CloneOpts{
				Url:       "https://example.com/repo.git",
				Ref:       "develop",
				Depth:     1,
				Recursive: true,
			},
			Timestamp: timestamppb.Now(),
		}
		data, err := op.MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT: %v", err)
		}

		decoded := &s4wave_git.CreateGitRepoWizardOp{}
		if err := decoded.UnmarshalVT(data); err != nil {
			t.Fatalf("UnmarshalVT: %v", err)
		}
		if decoded.GetObjectKey() != "git/repo/rt-test" {
			t.Fatalf("expected object_key git/repo/rt-test, got %q", decoded.GetObjectKey())
		}
		if !decoded.GetClone() {
			t.Fatal("expected clone=true")
		}
		if decoded.GetCloneOpts().GetUrl() != "https://example.com/repo.git" {
			t.Fatalf("expected clone URL, got %q", decoded.GetCloneOpts().GetUrl())
		}
		if decoded.GetCloneOpts().GetDepth() != 1 {
			t.Fatalf("expected depth 1, got %d", decoded.GetCloneOpts().GetDepth())
		}
		if !decoded.GetCloneOpts().GetRecursive() {
			t.Fatal("expected recursive=true")
		}
	})

	t.Run("wizard-lifecycle", func(t *testing.T) {
		_, engine, cleanup := setupWizardWorldEngine(ctx, t)
		defer cleanup()

		wizardKey := "wizard/git/repo/lifecycle-1"

		// Create wizard object for git/repo.
		wizardOp := s4wave_wizard.NewCreateWizardObjectOp(
			wizardKey, "wizard/git/repo",
			"git/repo", "git/repo/", "my-repo", time.Now(),
		)
		wizardData, err := wizardOp.MarshalVT()
		if err != nil {
			t.Fatalf("MarshalVT(wizard): %v", err)
		}

		tx, err := engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction(create): %v", err)
		}
		_, _, err = tx.ApplyWorldOp(ctx, s4wave_wizard.CreateWizardObjectOpId, wizardData, "")
		if err != nil {
			tx.Release()
			t.Fatalf("ApplyWorldOp(create wizard): %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit(create): %v", err)
		}
		tx.Release()

		// Verify wizard exists.
		readTx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction(verify create): %v", err)
		}
		_, found, err := readTx.GetObject(ctx, wizardKey)
		if err != nil {
			readTx.Release()
			t.Fatalf("GetObject(wizard): %v", err)
		}
		if !found {
			readTx.Release()
			t.Fatal("wizard object not found after create")
		}
		readTx.Release()

		// Delete wizard (simulating finalize cleanup).
		tx, err = engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatalf("NewTransaction(delete): %v", err)
		}
		_, err = tx.DeleteObject(ctx, wizardKey)
		if err != nil {
			tx.Release()
			t.Fatalf("DeleteObject(wizard): %v", err)
		}
		if err := tx.Commit(ctx); err != nil {
			tx.Release()
			t.Fatalf("Commit(delete): %v", err)
		}
		tx.Release()

		// Verify wizard deleted.
		readTx, err = engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction(verify delete): %v", err)
		}
		defer readTx.Release()
		_, found, err = readTx.GetObject(ctx, wizardKey)
		if err != nil {
			t.Fatalf("GetObject(wizard after delete): %v", err)
		}
		if found {
			t.Fatal("wizard object should be deleted")
		}
	})
}
