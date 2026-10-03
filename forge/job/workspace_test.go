package forge_job_test

import (
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_parent "github.com/s4wave/spacewave/db/world/parent"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_allocation "github.com/s4wave/spacewave/forge/lib/git/allocation"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// TestJobWorkspaceSurvivesExecutions verifies retained checkout custody through
// production Pass creation and explicit Job settlement.
func TestJobWorkspaceSurvivesExecutions(t *testing.T) {
	// Open the real World substrate without starting schedulers.
	ctx := t.Context()
	tb, err := hydra_testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()), hydra_testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	wtb, err := world_testbed.NewTestbed(tb, world_testbed.WithWorldVerbose(false))
	if err != nil {
		tb.Release()
		t.Fatal(err)
	}
	t.Cleanup(wtb.Release)

	// Create the Job before attaching its retained workspace.
	ws := world.NewEngineWorldState(wtb.Engine, true)
	sender, jobKey := tb.Volume.GetPeerID(), "forge/job/workspace"
	state, _, err := forge_job.CreateJobWithTasks(ctx, ws, sender, jobKey, nil, sender, nil, timestamp.Now())
	world.ReleaseObjectState(state)
	if err != nil {
		t.Fatal(err)
	}

	// Allocate private checkout objects over a shared repository object.
	args := forge_allocation.CreateArgs{JobObjectKey: jobKey, PeerID: sender.String(), RepoObjectKey: "git/repo/shared", WorktreeObjectKey: "git/worktree/retained", WorkdirObjectKey: "unixfs/workdir/retained", BaseCommitHash: "1111111111111111111111111111111111111111", PathFamily: "workspace"}
	for _, key := range []string{args.RepoObjectKey, args.WorktreeObjectKey, args.WorkdirObjectKey} {
		state, err := ws.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(state)
		if err != nil {
			t.Fatal(err)
		}
	}
	allocation, allocationKey, _, err := forge_allocation.CreateOrReuse(ctx, ws, args)
	if err != nil {
		t.Fatal(err)
	}
	if allocation.GetExecutionObjectKey() != "" || allocation.GetJobObjectKey() != jobKey || allocation.GetBranchRef() != "" {
		t.Fatalf("unexpected retained allocation: %+v", allocation)
	}

	// Job ownership round-trips and rejects ambiguous or absent owner kinds.
	loaded, err := forge_allocation.Lookup(ctx, ws, allocationKey)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.GetJobObjectKey() != jobKey || loaded.PeerID != sender.String() {
		t.Fatalf("lost Job grant: %+v", loaded)
	}
	for _, invalid := range []forge_allocation.CreateArgs{{}, {JobObjectKey: jobKey, ExecutionObjectKey: "execution/ambiguous"}, {JobObjectKey: jobKey, PassObjectKey: "pass/ambiguous"}} {
		invalid.RepoObjectKey, invalid.WorktreeObjectKey, invalid.BaseCommitHash, invalid.PathFamily = args.RepoObjectKey, args.WorktreeObjectKey, args.BaseCommitHash, args.PathFamily
		invalid.PeerID, invalid.WorkdirObjectKey = args.PeerID, args.WorkdirObjectKey
		if _, _, _, err := forge_allocation.CreateOrReuse(ctx, ws, invalid); err == nil {
			t.Fatalf("accepted ambiguous owner: %+v", invalid)
		}
	}

	// Each Pass uses the same Job allocation without transferring its lifetime.
	for _, name := range []string{"first", "second"} {
		taskKey, passKey := jobKey+"/task/"+name, "forge/pass/"+name
		task, err := ws.CreateObject(ctx, taskKey, nil)
		world.ReleaseObjectState(task)
		if err != nil {
			t.Fatal(err)
		}
		if err := world_parent.SetObjectParent(ctx, ws, taskKey, jobKey, false); err != nil {
			t.Fatal(err)
		}
		pass, _, err := forge_pass.CreatePassWithTarget(ctx, ws, sender, passKey, forge_target.NewValueSet(), &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}, 1, 1, sender.String(), nil, timestamp.Now())
		if err != nil {
			world.ReleaseObjectState(pass)
			t.Fatal(err)
		}
		if err := world_parent.SetObjectParent(ctx, ws, passKey, taskKey, false); err != nil {
			world.ReleaseObjectState(pass)
			t.Fatal(err)
		}
		executionKey := passKey + "/execution"
		_, _, err = world.AccessObjectState(ctx, pass, false, func(bcs *block.Cursor) error {
			body, err := forge_pass.UnmarshalPass(ctx, bcs)
			if err != nil {
				return err
			}
			_, err = forge_pass.CreateExecutionWithPass(ctx, ws, sender, executionKey, passKey, bcs, body, sender)
			return err
		})
		world.ReleaseObjectState(pass)
		if err != nil {
			t.Fatal(err)
		}
		keys, err := forge_allocation.ListExecutionAllocations(ctx, ws, executionKey)
		if err != nil || len(keys) != 1 || keys[0] != allocationKey {
			t.Fatalf("Execution binding: %v, %v", keys, err)
		}

		// Settlement must retain files while a bound Execution can still write.
		if err := forge_job.Settle(ctx, ws, jobKey); err == nil {
			t.Fatal("settlement accepted an active workspace Execution")
		}

		// Completing a turn leaves the Job's allocation and its checkout readable.
		_, _, err = world.AccessWorldObject(ctx, ws, executionKey, true, func(bcs *block.Cursor) error {
			// Complete only this Execution, preserving its Job allocation.
			execution, err := forge_execution.UnmarshalExecution(ctx, bcs)
			if err != nil {
				return err
			}
			execution.ExecutionState = forge_execution.State_ExecutionState_COMPLETE
			bcs.SetBlock(execution, true)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		retained, err := forge_allocation.Lookup(ctx, ws, allocationKey)
		if err != nil || retained.GetCleanupState() != "active" {
			t.Fatalf("turn released checkout: %+v, %v", retained, err)
		}
	}

	// Explicit settlement removes only private checkout state and is repeatable.
	for range 2 {
		if err := forge_job.Settle(ctx, ws, jobKey); err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range []string{args.WorktreeObjectKey, args.WorkdirObjectKey} {
		state, found, err := ws.GetObject(ctx, key)
		world.ReleaseObjectState(state)
		if err != nil || found {
			t.Fatalf("settled workspace remains: %s, %v", key, err)
		}
	}
	repo, found, err := ws.GetObject(ctx, args.RepoObjectKey)
	world.ReleaseObjectState(repo)
	if err != nil || !found {
		t.Fatalf("settlement removed shared Repo: %v", err)
	}
	retained, err := forge_allocation.Lookup(ctx, ws, allocationKey)
	if err != nil || retained.GetCleanupState() != "released" {
		t.Fatalf("release did not persist: %+v, %v", retained, err)
	}
	if _, _, err := forge_allocation.ResolveMount(ctx, ws, allocationKey); err == nil {
		t.Fatal("settled allocation grants a mount")
	}
}
