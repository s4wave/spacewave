//go:build !js

package wasm

import (
	"context"
	"testing"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	hydra_world "github.com/s4wave/spacewave/db/world"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_task "github.com/s4wave/spacewave/forge/task"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	sdk_world_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

type mountedForgeSpace struct {
	eng             *sdk_world_engine.SDKEngine
	engWs           hydra_world.WorldState
	sharedObjectRef resource_client.ResourceRef
	spaceRef        resource_client.ResourceRef
	contentsRef     resource_client.ResourceRef
	contentsSvc     s4wave_space.SRPCSpaceContentsResourceServiceClient
}

func mountForgeSpace(
	ctx context.Context,
	t testing.TB,
	sess *TestSession,
	sessionIndex uint32,
	spaceID string,
) *mountedForgeSpace {
	t.Helper()

	sessionSDK, err := sess.MountSessionByIdx(ctx, sessionIndex)
	if err != nil {
		t.Fatalf("MountSessionByIdx: %v", err)
	}
	defer sessionSDK.Release()

	mountResp, err := sessionSDK.MountSharedObject(ctx, spaceID)
	if err != nil {
		t.Fatalf("MountSharedObject: %v", err)
	}

	sharedObjectRef := sess.ResourceClient().CreateResourceReference(mountResp.GetResourceId())
	sharedObjectSrpcClient, err := sharedObjectRef.GetClient()
	if err != nil {
		sharedObjectRef.Release()
		t.Fatalf("GetClient(shared object): %v", err)
	}

	sharedObjectSvc := s4wave_sobject.NewSRPCSharedObjectResourceServiceClient(sharedObjectSrpcClient)
	bodyResp, err := sharedObjectSvc.MountSharedObjectBody(ctx, &s4wave_sobject.MountSharedObjectBodyRequest{})
	if err != nil {
		sharedObjectRef.Release()
		t.Fatalf("MountSharedObjectBody: %v", err)
	}

	spaceRef := sess.ResourceClient().CreateResourceReference(bodyResp.GetResourceId())
	spaceSrpcClient, err := spaceRef.GetClient()
	if err != nil {
		spaceRef.Release()
		sharedObjectRef.Release()
		t.Fatalf("GetClient(space): %v", err)
	}

	spaceSvc := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceSrpcClient)
	accessWorldResp, err := spaceSvc.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		spaceRef.Release()
		sharedObjectRef.Release()
		t.Fatalf("AccessWorld: %v", err)
	}

	engineRef := sess.ResourceClient().CreateResourceReference(accessWorldResp.GetResourceId())
	eng, err := sdk_world_engine.NewSDKEngine(sess.ResourceClient(), engineRef)
	if err != nil {
		engineRef.Release()
		spaceRef.Release()
		sharedObjectRef.Release()
		t.Fatalf("NewEngine: %v", err)
	}

	contentsResp, err := spaceSvc.MountSpaceContents(ctx, &s4wave_space.MountSpaceContentsRequest{})
	if err != nil {
		eng.Release()
		spaceRef.Release()
		sharedObjectRef.Release()
		t.Fatalf("MountSpaceContents: %v", err)
	}

	contentsRef := sess.ResourceClient().CreateResourceReference(contentsResp.GetResourceId())
	contentsSrpcClient, err := contentsRef.GetClient()
	if err != nil {
		contentsRef.Release()
		eng.Release()
		spaceRef.Release()
		sharedObjectRef.Release()
		t.Fatalf("GetClient(contents): %v", err)
	}

	return &mountedForgeSpace{
		eng:             eng,
		engWs:           hydra_world.NewEngineWorldState(eng, false),
		sharedObjectRef: sharedObjectRef,
		spaceRef:        spaceRef,
		contentsRef:     contentsRef,
		contentsSvc:     s4wave_space.NewSRPCSpaceContentsResourceServiceClient(contentsSrpcClient),
	}
}

func (m *mountedForgeSpace) Release() {
	if m.contentsRef != nil {
		m.contentsRef.Release()
	}
	if m.eng != nil {
		m.eng.Release()
	}
	if m.spaceRef != nil {
		m.spaceRef.Release()
	}
	if m.sharedObjectRef != nil {
		m.sharedObjectRef.Release()
	}
}

func listLinkedObjectKeys(
	ctx context.Context,
	tx hydra_world.WorldState,
	predicate string,
	subjectKeys ...string,
) ([]string, error) {
	var out []string
	for _, subjectKey := range subjectKeys {
		gqs, err := tx.LookupGraphQuads(
			ctx,
			hydra_world.NewGraphQuad(
				hydra_world.KeyToGraphValue(subjectKey).String(),
				predicate,
				"",
				"",
			),
			0,
		)
		if err != nil {
			return nil, err
		}
		for _, gq := range gqs {
			key, err := hydra_world.GraphValueToKey(gq.GetObj())
			if err != nil {
				return nil, err
			}
			out = append(out, key)
		}
	}
	return out, nil
}

func assertNoForgePasses(
	ctx context.Context,
	t testing.TB,
	engine hydra_world.Engine,
	jobKey string,
) {
	t.Helper()

	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	defer tx.Discard()

	taskKeys, err := listLinkedObjectKeys(ctx, tx, forge_job.PredJobToTask.String(), jobKey)
	if err != nil {
		t.Fatalf("ListJobTasks: %v", err)
	}
	for _, taskKey := range taskKeys {
		passKeys, err := listLinkedObjectKeys(ctx, tx, forge_task.PredTaskToPass.String(), taskKey)
		if err != nil {
			t.Fatalf("ListTaskPasses(%s): %v", taskKey, err)
		}
		if len(passKeys) != 0 {
			t.Fatalf("expected no passes before worker approval, got %v for %s", passKeys, taskKey)
		}
	}
}
