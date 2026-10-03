package forge_execution_test

import (
	"slices"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	pass_tx "github.com/s4wave/spacewave/forge/pass/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_testbed "github.com/s4wave/spacewave/forge/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/identity"
)

// TestExecutionPlacementWorkerToJob verifies placement maintains the sole Worker
// edge and the reverse graph path through Execution, Pass, Task, and Job.
func TestExecutionPlacementWorkerToJob(t *testing.T) {
	// Start the in-memory Forge World with its production operation lookup.
	ctx := t.Context()
	tb, err := forge_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	op := world.NewLookupOpController("forge-ops", tb.EngineID, forge_world.LookupWorldOp)
	release, err := tb.Bus.AddController(ctx, op, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Link two Workers to the same peer so replacement must follow the Worker key.
	peerID := tb.Volume.GetPeerID()
	publicKey, err := peerID.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	keypair, err := identity.NewKeypair(publicKey, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	workerKeys := []string{"forge/worker/first", "forge/worker/second"}
	for _, key := range workerKeys {
		if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, key, "worker", []*identity.Keypair{keypair}, peerID); err != nil {
			t.Fatal(err)
		}
	}

	// Create a placed Job and Task in one caller-owned transaction.
	const jobKey = "forge/job/placed"
	placement := &forge_worker.Placement{WorkerObjectKey: workerKeys[0], PeerId: peerID.String()}
	target := &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}
	ts := timestamp.Now()
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)

	// Store the Job and its Task, releasing the acquired object before committing.
	jobObject, _, err := forge_job.CreateJobWithTasks(ctx, tx, peerID, jobKey,
		map[string]*forge_target.Target{"run": target}, peerID, placement, ts)
	world.ReleaseObjectState(jobObject)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Resolve the Task's inputs and start its first Pass through production operations.
	taskKey := forge_job.NewJobTaskKey(jobKey, "run")
	update := task_tx.NewTxUpdateInputs(taskKey)
	update.TxUpdateInputs.UpdateTarget = true
	update.TxUpdateInputs.ResetInputs = true
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, update, peerID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, task_tx.NewTxStart(taskKey, true), peerID); err != nil {
		t.Fatal(err)
	}

	// Start the Pass through its production operation to create the placed Execution.
	passKey := forge_task.NewPassKey(taskKey, 1)
	start := pass_tx.NewTxStart(passKey, []*pass_tx.ExecSpec{{PeerId: peerID.String()}}, true)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, start, peerID); err != nil {
		t.Fatal(err)
	}
	executionKey := forge_pass.BuildPassExecutionObjKey(passKey, peerID.String())

	// Follow every reverse Forge relationship and retain the exact traversed quads.
	query := &world.GraphPathQuery{
		Steps: []world.GraphPathStep{
			{Direction: world.GraphPathDirectionIn, Predicate: forge_execution.PredExecutionToWorker.String(), Limit: 2},
			{Direction: world.GraphPathDirectionIn, Predicate: forge_pass.PredPassToExecution.String(), Limit: 2},
			{Direction: world.GraphPathDirectionIn, Predicate: forge_task.PredTaskToPass.String(), Limit: 2},
			{Direction: world.GraphPathDirectionIn, Predicate: forge_job.PredJobToTask.String(), Limit: 2},
		},
		ResultLimit:  2,
		IncludeQuads: true,
	}

	// Check initial placement, repeated placement, replacement, removal, and reassignment.
	for i, workerKey := range []string{workerKeys[0], workerKeys[0], workerKeys[1], "", workerKeys[0]} {
		// Recreate the Execution through its sole placement writer after the initial Pass start.
		var next *forge_worker.Placement
		if workerKey != "" {
			next = &forge_worker.Placement{WorkerObjectKey: workerKey, PeerId: peerID.String()}
		}
		if i != 0 {
			// Commit the Execution body and Worker edge in the same World transaction.
			tx, err := tb.Engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tx.Discard)
			if _, err := forge_execution.CreateExecutionWithTarget(ctx, tx, peerID, executionKey,
				peerID, forge_target.NewValueSet(), target, next, ts); err != nil {
				t.Fatal(err)
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}

		// Require the stored Execution placement to match the requested replacement.
		execution, object, err := forge_execution.LookupExecution(ctx, tb.WorldState, executionKey)
		world.ReleaseObjectState(object)
		if err != nil {
			t.Fatal(err)
		}
		if !execution.GetPlacement().EqualVT(next) {
			t.Fatalf("step %d: Execution placement = %v, want %v", i, execution.GetPlacement(), next)
		}

		// Require exactly one unlabeled Worker quad for a placed Execution.
		quads, err := tb.WorldState.LookupGraphQuads(ctx, forge_execution.NewExecutionToWorkerQuad(executionKey, ""), 0)
		if err != nil {
			t.Fatal(err)
		}
		wantQuads := 0
		if next != nil {
			wantQuads = 1
		}
		if len(quads) != wantQuads {
			t.Fatalf("step %d: Worker edges = %d, want %d", i, len(quads), wantQuads)
		}
		if next != nil && !world.GraphQuadToQuad(quads[0]).EqualVT(world.GraphQuadToQuad(forge_execution.NewExecutionToWorkerQuad(executionKey, workerKey))) {
			t.Fatalf("step %d: Worker quad = %v", i, quads[0])
		}

		// Require only the selected Worker to reach the exact Execution, Pass, Task, and Job.
		for _, key := range workerKeys {
			// Execute the complete graph traversal from this Worker.
			query.StartKeys = []string{key}
			result, err := tb.WorldState.QueryGraphPath(ctx, query)
			if err != nil {
				t.Fatal(err)
			}

			// Reject a stale path from an unselected or removed Worker.
			if key != workerKey {
				if len(result.ObjectKeys) != 0 || len(result.Quads) != 0 {
					t.Fatalf("step %d: unselected Worker %s reaches %v", i, key, result)
				}
				continue
			}

			// Verify the final Job and each intermediate Forge edge.
			if !slices.Equal(result.ObjectKeys, []string{jobKey}) {
				t.Fatalf("step %d: Worker reaches %v, want %s", i, result.ObjectKeys, jobKey)
			}
			want := []world.GraphQuad{
				forge_execution.NewExecutionToWorkerQuad(executionKey, key),
				forge_pass.NewPassToExecutionQuad(passKey, executionKey),
				forge_task.NewTaskToPassQuad(taskKey, passKey, 1),
				forge_job.NewJobToTaskQuad(jobKey, taskKey),
			}
			if len(result.Quads) != len(want) {
				t.Fatalf("step %d: traversed %d quads, want %d", i, len(result.Quads), len(want))
			}
			for _, quad := range want {
				if !slices.ContainsFunc(result.Quads, func(got world.GraphQuad) bool {
					return world.GraphQuadToQuad(got).EqualVT(world.GraphQuadToQuad(quad))
				}) {
					t.Fatalf("step %d: traversal missing %v", i, quad)
				}
			}
		}
	}
}

// TestBindExecutionWorkerUsesAssignedPeer permits an unpinned Worker to bind an
// Execution while retaining exclusive custody against a second eligible Worker.
func TestBindExecutionWorkerUsesAssignedPeer(t *testing.T) {
	// Open the real World used by the execution writer.
	ctx := t.Context()
	tb, err := forge_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Link two eligible Workers to the Execution peer.
	peerID := tb.Volume.GetPeerID()
	publicKey, err := peerID.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	keypair, err := identity.NewKeypair(publicKey, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"worker/first", "worker/second"} {
		if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, key, "worker", []*identity.Keypair{keypair}, peerID); err != nil {
			t.Fatal(err)
		}
	}

	// Bind an unplaced Execution, then repeat and challenge the same custody.
	for i, key := range []string{"worker/first", "worker/first", "worker/second"} {
		// Mutate custody in a caller-owned write transaction.
		tx, err := tb.Engine.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(tx.Discard)
		if i == 0 {
			if _, err := forge_execution.CreateExecutionWithTarget(ctx, tx, peerID, "execution/unplaced",
				peerID, forge_target.NewValueSet(), &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}, nil, timestamp.Now()); err != nil {
				t.Fatal(err)
			}
		}
		bound, err := forge_execution.BindExecutionWorker(ctx, tx, "execution/unplaced", &forge_worker.Placement{WorkerObjectKey: key})
		if err != nil {
			t.Fatal(err)
		}
		if bound != (i < 2) {
			t.Fatalf("binding %s = %v", key, bound)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Require the recorded peer and sole Worker edge to survive competing custody.
	execution, object, err := forge_execution.LookupExecution(ctx, tb.WorldState, "execution/unplaced")
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}
	if execution.GetPlacement().GetWorkerObjectKey() != "worker/first" || execution.GetPlacement().GetPeerId() != peerID.String() {
		t.Fatalf("placement = %v", execution.GetPlacement())
	}
	quads, err := tb.WorldState.LookupGraphQuads(ctx, forge_execution.NewExecutionToWorkerQuad("execution/unplaced", ""), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(quads) != 1 || !world.GraphQuadToQuad(quads[0]).EqualVT(world.GraphQuadToQuad(forge_execution.NewExecutionToWorkerQuad("execution/unplaced", "worker/first"))) {
		t.Fatalf("Worker edges = %v", quads)
	}
}
