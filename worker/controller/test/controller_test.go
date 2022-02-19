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
	"github.com/aperturerobotics/timestamp"
)

// TestWorkerController_Simple tests basic mechanics of the worker controller.
func TestWorkerController(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.Default(ctx)
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
	outState, err := tb.RunWorkerWithTasks(taskMap, nil, 1, &ts)
	if err != nil {
		t.Fatal(err.Error())
	}
	if outState.GetJobState() != forge_job.State_JobState_COMPLETE {
		t.Fatalf("expected job state COMPLETE but got %s", outState.GetJobState().String())
	}
}
