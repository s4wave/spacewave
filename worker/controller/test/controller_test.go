package worker_controller_testing

import (
	"context"
	"testing"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	forge_job "github.com/aperturerobotics/forge/job"
	forge_lib_kvtx "github.com/aperturerobotics/forge/lib/kvtx"
	forge_target "github.com/aperturerobotics/forge/target"
	target_mock "github.com/aperturerobotics/forge/target/mock"
	"github.com/aperturerobotics/forge/testbed"
	world_testbed "github.com/aperturerobotics/hydra/world/testbed"
	"github.com/aperturerobotics/timestamp"
)

// TestWorkerController tests basic mechanics of the worker controller.
func TestWorkerController(t *testing.T) {
	ctx := context.Background()

	verbose := true
	tb, err := testbed.Default(ctx, world_testbed.WithWorldVerbose(verbose))
	if err != nil {
		t.Fatal(err.Error())
	}

	// add the boilerplate controller factory
	// referenced in the Target below
	b, sr := tb.Bus, tb.StaticResolver
	sr.AddFactory(boilerplate_controller.NewFactory(b))
	sr.AddFactory(forge_lib_kvtx.NewFactory(b))

	// End to end test of building a target and running in a testbed.
	tgt, err := target_mock.ResolveMockTarget(ctx, b)
	if err != nil {
		t.Fatal(err.Error())
	}

	ts := timestamp.Now()
	taskMap := map[string]*forge_target.Target{
		"test-task": tgt,
	}
	jobKey := "job/1"
	clusterKey := "cluster/1"
	outState, err := tb.RunWorkerWithTasks(taskMap, nil, 1, ts, jobKey, clusterKey, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if outState.GetJobState() != forge_job.State_JobState_COMPLETE {
		t.Fatalf("expected job state COMPLETE but got %s", outState.GetJobState().String())
	}

	// lookup the results of the tasks
	jobTasks, _, err := forge_job.CollectJobTasks(ctx, tb.WorldState, "job/1")
	if err != nil {
		t.Fatal(err.Error())
	}
	le := tb.Logger
	le.Infof("job completed with %d tasks", len(jobTasks))
	for i, task := range jobTasks {
		le.Infof("completed_tasks[%d]: %v: pass %d", i, task.GetName(), task.GetPassNonce())
	}
}
