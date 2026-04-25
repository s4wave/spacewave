//go:build !js

package wasm

import (
	"context"
	"testing"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	"github.com/s4wave/spacewave/db/block"
	hydra_world "github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_task "github.com/s4wave/spacewave/forge/task"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

type mountedForgeSpace struct {
	engine      *s4wave_world.Engine
	spaceRef    resource_client.ResourceRef
	contentsRef resource_client.ResourceRef
	contentsSvc s4wave_space.SRPCSpaceContentsResourceServiceClient
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

	spaceRef := sess.ResourceClient().CreateResourceReference(mountResp.GetResourceId())
	spaceSrpcClient, err := spaceRef.GetClient()
	if err != nil {
		spaceRef.Release()
		t.Fatalf("GetClient(space): %v", err)
	}

	spaceSvc := s4wave_space.NewSRPCSpaceResourceServiceClient(spaceSrpcClient)
	accessWorldResp, err := spaceSvc.AccessWorld(ctx, &s4wave_space.AccessWorldRequest{})
	if err != nil {
		spaceRef.Release()
		t.Fatalf("AccessWorld: %v", err)
	}

	engineRef := sess.ResourceClient().CreateResourceReference(accessWorldResp.GetResourceId())
	engine, err := s4wave_world.NewEngine(sess.ResourceClient(), engineRef)
	if err != nil {
		engineRef.Release()
		spaceRef.Release()
		t.Fatalf("NewEngine: %v", err)
	}

	contentsResp, err := spaceSvc.MountSpaceContents(ctx, &s4wave_space.MountSpaceContentsRequest{})
	if err != nil {
		engine.Release()
		spaceRef.Release()
		t.Fatalf("MountSpaceContents: %v", err)
	}

	contentsRef := sess.ResourceClient().CreateResourceReference(contentsResp.GetResourceId())
	contentsSrpcClient, err := contentsRef.GetClient()
	if err != nil {
		contentsRef.Release()
		engine.Release()
		spaceRef.Release()
		t.Fatalf("GetClient(contents): %v", err)
	}

	return &mountedForgeSpace{
		engine:      engine,
		spaceRef:    spaceRef,
		contentsRef: contentsRef,
		contentsSvc: s4wave_space.NewSRPCSpaceContentsResourceServiceClient(contentsSrpcClient),
	}
}

func (m *mountedForgeSpace) Release() {
	if m.contentsRef != nil {
		m.contentsRef.Release()
	}
	if m.engine != nil {
		m.engine.Release()
	}
	if m.spaceRef != nil {
		m.spaceRef.Release()
	}
}

func listLinkedObjectKeys(
	ctx context.Context,
	tx *s4wave_world.Tx,
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

func lookupPassState(
	ctx context.Context,
	tx *s4wave_world.Tx,
	passKey string,
) (*forge_pass.Pass, error) {
	obj, found, err := tx.GetObject(ctx, passKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, hydra_world.ErrObjectNotFound
	}
	var pass *forge_pass.Pass
	_, _, err = hydra_world.AccessObjectState(
		ctx,
		obj,
		false,
		func(bcs *block.Cursor) error {
			var unmarshalErr error
			pass, unmarshalErr = forge_pass.UnmarshalPass(ctx, bcs)
			return unmarshalErr
		},
	)
	return pass, err
}

func lookupExecutionState(
	ctx context.Context,
	tx *s4wave_world.Tx,
	execKey string,
) (*forge_execution.Execution, error) {
	obj, found, err := tx.GetObject(ctx, execKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, hydra_world.ErrObjectNotFound
	}
	var execState *forge_execution.Execution
	_, _, err = hydra_world.AccessObjectState(
		ctx,
		obj,
		false,
		func(bcs *block.Cursor) error {
			var unmarshalErr error
			execState, unmarshalErr = forge_execution.UnmarshalExecution(ctx, bcs)
			return unmarshalErr
		},
	)
	return execState, err
}

func assertNoForgePasses(
	ctx context.Context,
	t testing.TB,
	engine *s4wave_world.Engine,
	jobKey string,
) {
	t.Helper()

	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatalf("NewTransaction: %v", err)
	}
	defer tx.Discard(ctx)

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

func waitForForgeExecution(
	ctx context.Context,
	t testing.TB,
	engine *s4wave_world.Engine,
	jobKey string,
) (string, string, *forge_pass.Pass, *forge_execution.Execution) {
	t.Helper()

	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatalf("GetSeqno: %v", err)
	}

	for {
		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatalf("NewTransaction: %v", err)
		}

		taskKeys, err := listLinkedObjectKeys(
			ctx,
			tx,
			forge_job.PredJobToTask.String(),
			jobKey,
		)
		if err != nil {
			_ = tx.Discard(ctx)
			t.Fatalf("ListJobTasks: %v", err)
		}

		for _, taskKey := range taskKeys {
			passKeys, err := listLinkedObjectKeys(
				ctx,
				tx,
				forge_task.PredTaskToPass.String(),
				taskKey,
			)
			if err != nil {
				_ = tx.Discard(ctx)
				t.Fatalf("ListTaskPasses(%s): %v", taskKey, err)
			}
			for _, passKey := range passKeys {
				passState, err := lookupPassState(ctx, tx, passKey)
				if err != nil {
					_ = tx.Discard(ctx)
					t.Fatalf("LookupPass(%s): %v", passKey, err)
				}

				execKeys, err := listLinkedObjectKeys(
					ctx,
					tx,
					forge_pass.PredPassToExecution.String(),
					passKey,
				)
				if err != nil {
					_ = tx.Discard(ctx)
					t.Fatalf("ListPassExecutions(%s): %v", passKey, err)
				}
				for _, execKey := range execKeys {
					execState, err := lookupExecutionState(ctx, tx, execKey)
					if err != nil {
						_ = tx.Discard(ctx)
						t.Fatalf("LookupExecution(%s): %v", execKey, err)
					}
					if passState.IsComplete() && execState.IsComplete() && len(execState.GetLogEntries()) != 0 {
						if err := tx.Discard(ctx); err != nil {
							t.Fatalf("Discard(tx): %v", err)
						}
						return passKey, execKey, passState, execState
					}
				}
			}
		}

		if err := tx.Discard(ctx); err != nil {
			t.Fatalf("Discard(tx): %v", err)
		}

		seqno, err = engine.WaitSeqno(ctx, seqno+1)
		if err != nil {
			t.Fatalf("WaitSeqno: %v", err)
		}
	}
}
