package worker_controller_test

import (
	"context"
	"slices"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/forge/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
)

// TestWorkerRetiresExecutionAfterFinalResult exercises retirement requested
// before a pending Task creates its Pass and Execution and writes final results.
func TestWorkerRetiresExecutionAfterFinalResult(t *testing.T) {
	// Seed pending Task demand and the persistent retirement request.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	workerPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	sender := workerPeer.GetPeerID()

	// Request retirement before the worker admits any attempts.
	const taskKey = "task/retirement"
	object, _, err := forge_task.CreateTaskWithTarget(ctx, tb.WorldState, sender, taskKey, "retirement", &forge_target.Target{Exec: &forge_target.Exec{Disable: true}}, sender, 1, nil, timestamp.Now())
	world.ReleaseObjectState(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := forge_task.RetireTask(ctx, tb.WorldState, taskKey, sender); err != nil {
		t.Fatal(err)
	}

	// Supply the actual Worker peer and World operation resolver.
	keypair, err := identity.NewKeypair(workerPeer.GetPubKey(), "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, "worker/retirement", "retirement", []*identity.Keypair{keypair}, sender); err != nil {
		t.Fatal(err)
	}
	releasePeer, err := tb.Bus.AddController(ctx, peer_controller.NewController(tb.Logger, workerPeer), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releasePeer)

	// Register Forge operation decoding for the real execution controller.
	releaseOps, err := tb.Bus.AddController(ctx, world.NewLookupOpController("retirement-ops", tb.EngineID, forge_world.LookupWorldOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseOps)

	// Retain real Worker demand while the Execution writes its final result.
	_, reference, err := worker_controller.StartControllerWithConfig(ctx, tb.Bus, worker_controller.NewConfig(tb.EngineID, "worker/retirement", sender, true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reference.Release)
	keypairKey := identity_world.NewKeypairKey(sender.String())
	for {
		// Observe assignment and its notification cursor in one World snapshot.
		var seqno uint64
		var assigned []string
		if err := world.ExecTransaction(ctx, tb.Engine, false, func(ctx context.Context, ws world.WorldState) error {
			// Read worker demand and its change cursor atomically.
			var err error
			seqno, err = ws.GetSeqno(ctx)
			if err != nil {
				return err
			}
			assigned, err = forge_world.ListKeypairObjects(ctx, ws, keypairKey)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		if slices.Equal(assigned, []string{"worker/retirement"}) {
			break
		}

		// Wait for the owning World's revision, without polling completion.
		if _, err := tb.Engine.WaitSeqno(ctx, seqno+1); err != nil {
			t.Fatal(err)
		}
	}

	// Retirement includes attempts created after the initial request.
	passes, err := forge_task.ListTaskPasses(ctx, tb.WorldState, taskKey)
	if err != nil || len(passes) != 1 {
		t.Fatalf("retired Pass history = %v, %v", passes, err)
	}
	executions, err := forge_pass.ListPassExecutions(ctx, tb.WorldState, passes[0])
	if err != nil || len(executions) != 1 {
		t.Fatalf("retired Execution history = %v, %v", executions, err)
	}

	// Detachment requires the durable result and preserves assigned peer authority.
	body, err := world.LookupObjectBody[*forge_execution.Execution](ctx, tb.WorldState, executions[0], forge_execution.NewExecutionBlock)
	if err != nil {
		t.Fatal(err)
	}
	if !body.IsComplete() || !body.GetResult().GetSuccess() || body.GetPeerId() != sender.String() {
		t.Fatalf("detached Execution lost its final result or peer: %v", body)
	}
}
