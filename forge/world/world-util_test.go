//go:build !js

package forge_world_test

import (
	"context"
	"slices"
	"testing"

	"github.com/aperturerobotics/cayley"
	resource_testbed "github.com/s4wave/spacewave/core/resource/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	forge_world "github.com/s4wave/spacewave/forge/world"
	identity_world "github.com/s4wave/spacewave/identity/world"
	s4wave_testbed "github.com/s4wave/spacewave/sdk/testbed"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

func TestListKeypairObjectsLocalSDKParity(t *testing.T) {
	ctx := context.Background()
	tb, resClient, cleanup := resource_testbed.SetupTestbedWithClient(ctx, t)
	defer cleanup()

	rootRef := resClient.AccessRootResource()
	defer rootRef.Release()
	srpcClient, err := rootRef.GetClient()
	if err != nil {
		t.Fatal(err.Error())
	}

	const engineID = "forge-keypair-objects-parity"
	testbedClient := s4wave_testbed.NewSRPCTestbedResourceServiceClient(srpcClient)
	createResp, err := testbedClient.CreateWorld(ctx, &s4wave_testbed.CreateWorldRequest{EngineId: engineID})
	if err != nil {
		t.Fatal(err.Error())
	}

	engineRef := resClient.CreateResourceReference(createResp.ResourceId)
	sdkEngine, err := sdk_world_engine.NewSDKEngine(resClient, engineRef)
	if err != nil {
		engineRef.Release()
		t.Fatal(err.Error())
	}
	defer sdkEngine.Release()

	writeTx, err := sdkEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	seedKeypairObjects(t, ctx, writeTx)
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}

	localEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	defer localEngine.ClearContext()
	localTx, err := localEngine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer localTx.Discard()

	keypairKeys := []string{"kp/beta", "kp/alpha"}
	legacy, err := listKeypairObjectsCayley(ctx, localTx, keypairKeys...)
	if err != nil {
		t.Fatal(err.Error())
	}
	expected := []string{
		"forge/cluster",
		"forge/execution",
		"forge/job",
		"forge/pass",
		"forge/task",
		"forge/worker",
	}
	if !slices.Equal(legacy, expected) {
		t.Fatalf("unexpected legacy result: got %v, want %v", legacy, expected)
	}

	local, err := forge_world.ListKeypairObjects(ctx, localTx, keypairKeys...)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Equal(local, legacy) {
		t.Fatalf("local result differs from legacy Cayley traversal: got %v, want %v", local, legacy)
	}

	readTx, err := sdkEngine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer readTx.Discard()
	remote := &countingWorldState{WorldState: readTx}
	remoteResult, err := forge_world.ListKeypairObjects(ctx, remote, keypairKeys...)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !slices.Equal(remoteResult, legacy) {
		t.Fatalf("SDK result differs from local Cayley traversal: got %v, want %v", remoteResult, legacy)
	}
	if remote.accessCayleyGraphCalls != 0 {
		t.Fatalf("expected zero AccessCayleyGraph calls, got %d", remote.accessCayleyGraphCalls)
	}
	if remote.queryGraphPathCalls != 1 {
		t.Fatalf("expected one QueryGraphPath call, got %d", remote.queryGraphPathCalls)
	}
	if remote.getObjectMetadataBatchCalls != 1 {
		t.Fatalf("expected one GetObjectMetadataBatch call, got %d", remote.getObjectMetadataBatchCalls)
	}

	noMatchLegacy, err := listKeypairObjectsCayley(ctx, localTx, "kp/gamma")
	if err != nil {
		t.Fatal(err.Error())
	}
	if noMatchLegacy != nil {
		t.Fatalf("expected nil legacy no-match result, got %v", noMatchLegacy)
	}
	noMatchLocal, err := forge_world.ListKeypairObjects(ctx, localTx, "kp/gamma")
	if err != nil {
		t.Fatal(err.Error())
	}
	if noMatchLocal != nil {
		t.Fatalf("expected nil local no-match result, got %v", noMatchLocal)
	}
	noMatchRemote, err := forge_world.ListKeypairObjects(ctx, remote, "kp/gamma")
	if err != nil {
		t.Fatal(err.Error())
	}
	if noMatchRemote != nil {
		t.Fatalf("expected nil SDK no-match result, got %v", noMatchRemote)
	}
	if remote.accessCayleyGraphCalls != 0 {
		t.Fatalf("expected zero AccessCayleyGraph calls after no-match query, got %d", remote.accessCayleyGraphCalls)
	}
}

type countingWorldState struct {
	world.WorldState
	accessCayleyGraphCalls      int
	queryGraphPathCalls         int
	getObjectMetadataBatchCalls int
}

func (w *countingWorldState) AccessCayleyGraph(ctx context.Context, write bool, cb func(context.Context, world.CayleyHandle) error) error {
	w.accessCayleyGraphCalls++
	return w.WorldState.AccessCayleyGraph(ctx, write, cb)
}

func (w *countingWorldState) QueryGraphPath(ctx context.Context, query *world.GraphPathQuery) (*world.GraphPathQueryResult, error) {
	w.queryGraphPathCalls++
	return w.WorldState.QueryGraphPath(ctx, query)
}

func (w *countingWorldState) GetObjectMetadataBatch(ctx context.Context, keys []string) ([]*world_types.ObjectMetadata, error) {
	w.getObjectMetadataBatchCalls++
	return world_types.GetObjectMetadataBatch(ctx, w.WorldState, keys)
}

func seedKeypairObjects(t *testing.T, ctx context.Context, ws world.WorldState) {
	t.Helper()

	objects := []struct {
		key    string
		typeID string
	}{
		{key: "forge/worker", typeID: forge_worker.WorkerTypeID},
		{key: "forge/task", typeID: forge_task.TaskTypeID},
		{key: "forge/pass", typeID: forge_pass.PassTypeID},
		{key: "forge/job", typeID: forge_job.JobTypeID},
		{key: "forge/execution", typeID: forge_execution.ExecutionTypeID},
		{key: "forge/cluster", typeID: forge_cluster.ClusterTypeID},
		{key: "identity/entity", typeID: identity_world.EntityTypeID},
	}
	for _, key := range []string{"kp/alpha", "kp/beta", "kp/gamma", "object/untyped"} {
		if _, err := ws.CreateObject(ctx, key, nil); err != nil {
			t.Fatal(err.Error())
		}
	}
	for _, obj := range objects {
		if _, err := ws.CreateObject(ctx, obj.key, nil); err != nil {
			t.Fatal(err.Error())
		}
		if err := world_types.SetObjectType(ctx, ws, obj.key, obj.typeID); err != nil {
			t.Fatal(err.Error())
		}
	}

	links := [][2]string{
		{"forge/worker", "kp/alpha"},
		{"forge/task", "kp/beta"},
		{"forge/pass", "kp/alpha"},
		{"forge/job", "kp/beta"},
		{"forge/execution", "kp/alpha"},
		{"forge/cluster", "kp/beta"},
		{"forge/cluster", "kp/alpha"},
		{"identity/entity", "kp/alpha"},
		{"identity/entity", "kp/gamma"},
		{"object/untyped", "kp/beta"},
	}
	for _, link := range links {
		if err := ws.SetGraphQuad(ctx, identity_world.NewObjectToKeypairQuad(link[0], link[1])); err != nil {
			t.Fatal(err.Error())
		}
	}
}

func listKeypairObjectsCayley(ctx context.Context, ws world.WorldState, keypairKeys ...string) ([]string, error) {
	return world.CollectPathWithKeys(
		ctx,
		ws,
		keypairKeys,
		func(p *cayley.Path) (*cayley.Path, error) {
			return world_types.LimitNodesToTypes(
				p.In(identity_world.PredObjectToKeypair),
				forge_world.ForgeObjectTypeIDs...,
			), nil
		},
	)
}
