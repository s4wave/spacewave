package forge_job_test

import (
	"testing"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	forge_job_ops "github.com/s4wave/spacewave/core/forge/job"
	"github.com/s4wave/spacewave/db/world"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	pass_controller "github.com/s4wave/spacewave/forge/pass/controller"
	pass_tx "github.com/s4wave/spacewave/forge/pass/tx"
	forge_task "github.com/s4wave/spacewave/forge/task"
	task_tx "github.com/s4wave/spacewave/forge/task/tx"
	forge_testbed "github.com/s4wave/spacewave/forge/testbed"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/identity"
	"github.com/s4wave/spacewave/net/peer"
)

// TestJobPlacementBindsExecutionPeer follows one selection through the Forge
// state transitions and rejects both an unlinked Device peer and another peer's
// execution spec.
func TestJobPlacementBindsExecutionPeer(t *testing.T) {
	// Start the Forge testbed and register the Job creation operations.
	ctx := t.Context()
	tb, err := forge_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	op := world.NewLookupOpController("forge-ops", tb.EngineID, world.NewLookupOpFromSlice([]world.LookupOp{
		forge_world.LookupWorldOp,
		forge_job_ops.LookupForgeJobCreateOp,
	}))
	release, err := tb.Bus.AddController(ctx, op, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Build the first Worker identity from the controller peer.
	controllerPeer := tb.Volume.GetPeerID()
	controllerPublic, err := controllerPeer.ExtractPublicKey()
	if err != nil {
		t.Fatal(err)
	}
	firstKeypair, err := identity.NewKeypair(controllerPublic, "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Build a distinct Device peer for the selected Worker.
	selectedPeer, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	selectedKeypair, err := identity.NewKeypair(selectedPeer.GetPubKey(), "", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create both Workers with their respective Device identities.
	for _, worker := range []struct {
		key, name string
		keypair   *identity.Keypair
	}{
		{"workers/first", "first", firstKeypair},
		{"workers/selected", "selected", selectedKeypair},
	} {
		if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, worker.key, worker.name,
			[]*identity.Keypair{worker.keypair}, controllerPeer); err != nil {
			t.Fatal(err)
		}
	}

	// Create the cluster that will contain the placed Job.
	const jobKey = "jobs/placed"
	const clusterKey = "clusters/placement"
	if _, _, err := forge_cluster.CreateCluster(ctx, tb.WorldState, clusterKey, "placement", controllerPeer, controllerPeer); err != nil {
		t.Fatal(err)
	}

	// Verify that Job creation rejects a peer linked to a different Worker.
	unlinked := &forge_worker.Placement{WorkerObjectKey: "workers/first", PeerId: selectedPeer.GetPeerID().String()}
	_, _, err = tb.WorldState.ApplyWorldOp(ctx, &forge_job_ops.ForgeJobCreateOp{
		JobKey: "jobs/rejected", ClusterKey: clusterKey,
		TaskDefs:  []*forge_job_ops.ForgeJobTaskDef{{Name: "build"}},
		Placement: unlinked, Timestamp: timestamp.Now(),
	}, controllerPeer)
	if err == nil {
		t.Fatal("accepted a Device peer not linked to the selected Worker")
	}

	// Create the Job with the selected Worker and its linked Device peer.
	placement := &forge_worker.Placement{WorkerObjectKey: "workers/selected", PeerId: selectedPeer.GetPeerID().String()}
	_, _, err = tb.WorldState.ApplyWorldOp(ctx, &forge_job_ops.ForgeJobCreateOp{
		JobKey: jobKey, ClusterKey: clusterKey,
		TaskDefs:  []*forge_job_ops.ForgeJobTaskDef{{Name: "build"}},
		Placement: placement, Timestamp: timestamp.Now(),
	}, controllerPeer)
	if err != nil {
		t.Fatal(err)
	}

	// Verify that the Job retains the selected placement.
	job, err := forge_job.LookupJobBody(ctx, tb.WorldState, jobKey)
	if err != nil {
		t.Fatal(err)
	}
	if !job.GetPlacement().EqualVT(placement) {
		t.Fatalf("Job placement = %v", job.GetPlacement())
	}

	// Verify that the Job Task inherits the selected placement.
	taskKey := forge_job.NewJobTaskKey(jobKey, "build")
	task, err := forge_task.LookupTaskBody(ctx, tb.WorldState, taskKey)
	if err != nil {
		t.Fatal(err)
	}
	if !task.GetPlacement().EqualVT(placement) {
		t.Fatalf("Task placement = %v", task.GetPlacement())
	}

	// Refresh the Task inputs and start its first Pass.
	update := task_tx.NewTxUpdateInputs(taskKey)
	update.TxUpdateInputs.UpdateTarget = true
	update.TxUpdateInputs.ResetInputs = true
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, update, controllerPeer); err != nil {
		t.Fatal(err)
	}
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, task_tx.NewTxStart(taskKey, true), controllerPeer); err != nil {
		t.Fatal(err)
	}

	// Verify that the Pass inherits the selected placement.
	passKey := forge_task.NewPassKey(taskKey, 1)
	pass, _, err := forge_pass.LookupPass(ctx, tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}
	if !pass.GetPlacement().EqualVT(placement) {
		t.Fatalf("Pass placement = %v", pass.GetPlacement())
	}

	// Verify that the Pass rejects an Execution on the other Worker peer.
	wrong := pass_tx.NewTxStart(passKey, []*pass_tx.ExecSpec{{PeerId: controllerPeer.String()}}, true)
	if _, _, err := tb.WorldState.ApplyWorldOp(ctx, wrong, controllerPeer); err == nil {
		t.Fatal("Pass accepted an Execution on the other Worker's peer")
	}

	// Open the Pass object and its committed root for controller execution.
	passObject, err := world.MustGetObject(ctx, tb.WorldState, passKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(passObject)
	rootRef, _, err := world.LookupRootRef(ctx, tb.Engine, passKey)
	if err != nil {
		t.Fatal(err)
	}

	// Run the Pass controller to create the selected peer Execution.
	controller := pass_controller.NewController(tb.Logger, tb.Bus,
		pass_controller.NewConfig(tb.EngineID, passKey, controllerPeer, true))
	t.Cleanup(func() {
		if err := controller.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := controller.ProcessState(ctx, tb.Logger, tb.WorldState, passObject, rootRef, 0); err != nil {
		t.Fatal(err)
	}

	// Verify that the Execution retains the selected placement and peer.
	executionKey := forge_pass.BuildPassExecutionObjKey(passKey, selectedPeer.GetPeerID().String())
	execution, executionObject, err := forge_execution.LookupExecution(ctx, tb.WorldState, executionKey)
	world.ReleaseObjectState(executionObject)
	if err != nil {
		t.Fatal(err)
	}
	if execution.GetPeerId() != placement.GetPeerId() || !execution.GetPlacement().EqualVT(placement) {
		t.Fatalf("Execution placement = %v, peer = %s", execution.GetPlacement(), execution.GetPeerId())
	}

	// Open the Execution object for peer claim checks.
	executionObject, err = world.MustGetObject(ctx, tb.WorldState, executionKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(executionObject)

	// Verify that the other Worker peer cannot claim the Execution.
	if _, _, err := executionObject.ApplyObjectOp(ctx,
		execution_tx.NewTxStart(controllerPeer, time.Now().Add(time.Hour), "wrong-peer"), controllerPeer); err == nil {
		t.Fatal("other Worker's peer claimed the selected Execution")
	}

	// Verify that the selected Device peer can claim the Execution.
	if _, _, err := executionObject.ApplyObjectOp(ctx,
		execution_tx.NewTxStart(selectedPeer.GetPeerID(), time.Now().Add(time.Hour), "selected-peer"), selectedPeer.GetPeerID()); err != nil {
		t.Fatalf("selected peer could not claim Execution: %v", err)
	}
}
