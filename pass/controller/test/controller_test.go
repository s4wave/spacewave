package pass_controller_testing

import (
	"context"
	"testing"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	forge_lib_kvtx "github.com/aperturerobotics/forge/lib/kvtx"
	forge_pass "github.com/aperturerobotics/forge/pass"
	target_mock "github.com/aperturerobotics/forge/target/mock"
	"github.com/aperturerobotics/forge/testbed"
	"github.com/aperturerobotics/timestamp"
)

// TestPassController_Simple tests basic mechanics of the pass controller.
func TestPassController(t *testing.T) {
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
	outState, err := tb.RunPassWithTarget(tgt, nil, 1, &ts)
	if err != nil {
		t.Fatal(err.Error())
	}
	if outState.GetPassState() != forge_pass.State_PassState_COMPLETE {
		t.Fatalf("expected pass state COMPLETE but got %s", outState.GetPassState().String())
	}
}
