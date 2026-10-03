package worker_controller_test

import (
	"testing"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_job "github.com/s4wave/spacewave/forge/job"
	forge_pass "github.com/s4wave/spacewave/forge/pass"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_task "github.com/s4wave/spacewave/forge/task"
	"github.com/s4wave/spacewave/forge/testbed"
)

// TestAutomaticExecutionWorkerToJob follows custody assigned by a real Worker,
// including Pass completion after an initially unplaced Execution is bound.
func TestAutomaticExecutionWorkerToJob(t *testing.T) {
	// Run an unplaced Job through the production Worker and Forge controllers.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	const jobKey = "job/automatic"
	result, err := tb.RunWorkerWithTasks(map[string]*forge_target.Target{
		"run": {Exec: &forge_target.Exec{Disable: true}},
	}, nil, 1, timestamp.Now(), jobKey, "cluster/automatic", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.GetResult().GetSuccess() {
		t.Fatalf("automatic Job result = %v", result.GetResult())
	}

	// Traverse the Worker's Execution, Pass, Task and Job through their real edges.
	query := &world.GraphPathQuery{
		StartKeys:   []string{"worker/1"},
		ResultLimit: 10,
		Steps: []world.GraphPathStep{
			{Direction: world.GraphPathDirectionIn, Predicate: forge_execution.PredExecutionToWorker.String(), Limit: 10},
			{Direction: world.GraphPathDirectionIn, Predicate: forge_pass.PredPassToExecution.String(), Limit: 10},
			{Direction: world.GraphPathDirectionIn, Predicate: forge_task.PredTaskToPass.String(), Limit: 10},
			{Direction: world.GraphPathDirectionIn, Predicate: forge_job.PredJobToTask.String(), Limit: 10},
		},
	}
	path, err := tb.WorldState.QueryGraphPath(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(path.ObjectKeys) != 1 || path.ObjectKeys[0] != jobKey {
		t.Fatalf("Worker-to-Job path = %v, want %s", path, jobKey)
	}
}
