package forge_cluster_test

import (
	"context"
	"crypto/rand"
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_cluster "github.com/s4wave/spacewave/forge/cluster"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	forge_value "github.com/s4wave/spacewave/forge/value"
	net_crypto "github.com/s4wave/spacewave/net/crypto"
	net_peer "github.com/s4wave/spacewave/net/peer"
)

// completeJobTestSetup builds a cluster with one linked job and one task.
func completeJobTestSetup(t *testing.T, ws world.WorldState, peerID net_peer.ID) (string, string, string) {
	t.Helper()
	ctx := context.Background()

	if _, _, err := forge_cluster.CreateCluster(ctx, ws, "cluster/test-cluster", "test-cluster", peerID, peerID); err != nil {
		t.Fatal(err)
	}

	jobKey := "job/test-job"
	if _, _, err := forge_job.CreateJobWithTasks(ctx, ws, peerID, jobKey, map[string]*forge_target.Target{
		"task-a": {Exec: &forge_target.Exec{Disable: true}},
	}, peerID, timestamp.Now()); err != nil {
		t.Fatal(err)
	}
	taskKey := forge_job.NewJobTaskKey(jobKey, "task-a")

	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(
		"cluster/test-cluster",
		forge_cluster.PredClusterToJob.String(),
		jobKey,
		"",
	)); err != nil {
		t.Fatal(err)
	}
	return "cluster/test-cluster", jobKey, taskKey
}

// setTaskResult marks the task COMPLETE with the given result.
func setTaskResult(t *testing.T, ws world.WorldState, taskKey string, res *forge_value.Result) {
	t.Helper()
	ctx := context.Background()
	_, _, err := world.AccessWorldObject(ctx, ws, taskKey, true, func(bcs *block.Cursor) error {
		task, err := forge_task.UnmarshalTask(ctx, bcs)
		if err != nil {
			return err
		}
		tgtObj, err := world.MustGetObject(ctx, ws, forge_task.NewTargetKey(taskKey))
		if err != nil {
			return err
		}
		tgtRef, _, err := tgtObj.GetRootRef(ctx)
		if err != nil {
			return err
		}
		task.TargetRef = tgtRef.GetRootRef()
		task.TaskState = forge_task.State_TaskState_COMPLETE
		task.Result = res
		if err := task.Validate(); err != nil {
			return err
		}
		bcs.SetBlock(task, true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestCompleteJobAggregatesFailedTask pins that completing a job records a
// failure when a linked task failed.
func TestCompleteJobAggregatesFailedTask(t *testing.T) {
	ctx := context.Background()
	wtb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()
	ws := world.NewEngineWorldState(wtb.Engine, true)

	sk, _, err := net_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := net_peer.IDFromPrivateKey(sk)
	if err != nil {
		t.Fatal(err)
	}

	clusterKey, jobKey, taskKey := completeJobTestSetup(t, ws, sender)
	setTaskResult(t, ws, taskKey, &forge_value.Result{Success: false, FailError: "boom"})

	if _, _, err := forge_cluster.CompleteJob(ctx, ws, clusterKey, jobKey, sender); err != nil {
		t.Fatal(err)
	}

	job, err := forge_job.LookupJobBody(ctx, ws, jobKey)
	if err != nil {
		t.Fatal(err)
	}
	if !job.IsComplete() {
		t.Fatal("expected job to be COMPLETE")
	}
	res := job.GetResult()
	if res.GetSuccess() {
		t.Fatalf("expected failed job result, got success=%v failError=%q", res.GetSuccess(), res.GetFailError())
	}
}
