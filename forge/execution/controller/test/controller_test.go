package execution_controller_testing

import (
	"context"
	"testing"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	target_mock "github.com/s4wave/spacewave/forge/target/mock"
	"github.com/s4wave/spacewave/forge/testbed"
)

// TestExecutionController_Simple tests basic mechanics of the execution controller.
func TestExecutionController(t *testing.T) {
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
	outState, err := tb.RunExecutionWithTarget(tgt, nil, ts)
	if err != nil {
		t.Fatal(err.Error())
	}
	if outState.GetExecutionState() != forge_execution.State_ExecutionState_COMPLETE {
		t.Fatalf("expected execution state COMPLETE but got %s", outState.GetExecutionState().String())
	}
}
