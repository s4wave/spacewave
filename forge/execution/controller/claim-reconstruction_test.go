package execution_controller

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	target_mock "github.com/s4wave/spacewave/forge/target/mock"
)

func TestReconstructedConfigResumesClaimBeforeTargetConstruction(t *testing.T) {
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	targetFactory := &countingFactory{inner: forge_lib_kvtx.NewFactory(tb.Bus)}
	tb.StaticResolver.AddFactory(targetFactory)

	opController := world.NewLookupOpController(
		"execution-reconstruction-tx-ops",
		tb.EngineID,
		execution_tx.LookupWorldOp,
	)
	opRelease, err := tb.Bus.AddController(ctx, opController, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(opRelease)

	target, err := target_mock.ResolveMockTarget(ctx, tb.Bus)
	if err != nil {
		t.Fatal(err)
	}
	peerID := tb.Volume.GetPeerID()
	objKey := "test/execution/controller-reconstruction"
	_, err = forge_execution.CreateExecutionWithTarget(
		ctx,
		tb.WorldState,
		peerID,
		objKey,
		peerID,
		forge_target.NewValueSet(),
		target,
		timestamp.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	obj, err := world.MustGetObject(ctx, tb.WorldState, objKey)
	if err != nil {
		t.Fatal(err)
	}

	buildConfig := func() *Config {
		conf := NewConfig(
			tb.EngineID,
			objKey,
			peerID,
			&forge_target.InputWorld{EngineId: tb.EngineID},
		)
		conf.AllowNonExecController = true
		return conf
	}
	process := func(ctrl *Controller) {
		t.Helper()
		rootRef, rev, err := obj.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err)
		}
		wait, err := ctrl.ProcessState(ctx, tb.Logger, tb.WorldState, obj, rootRef, rev)
		if err != nil {
			t.Fatal(err)
		}
		if !wait {
			t.Fatal("active execution controller stopped observing")
		}
	}

	firstConfig := buildConfig()
	first := NewController(tb.Logger, tb.Bus, firstConfig)
	firstCtx, stopFirst := context.WithCancel(ctx)
	first.busEngine.SetContext(firstCtx)
	first.execRoutine.SetContext(firstCtx, true)
	process(first)
	stopFirst()
	if first.execRoutine.GetState() != nil {
		t.Fatal("first controller reached the target before reconstruction")
	}
	if got := targetFactory.constructed.Load(); got != 0 {
		t.Fatalf("target construction count before reconstruction = %d, want 0", got)
	}

	running, _, err := forge_execution.LookupExecution(ctx, tb.WorldState, objKey)
	if err != nil {
		t.Fatal(err)
	}
	if got := running.GetExecutionState(); got != forge_execution.State_ExecutionState_RUNNING {
		t.Fatalf("execution state after first claim = %s, want RUNNING", got)
	}
	if got := running.GetClaim().GetClaimId(); got != firstConfig.GetClaimId() {
		t.Fatalf("durable claim ID = %q, want %q", got, firstConfig.GetClaimId())
	}

	replacementConfig := buildConfig()
	if replacementConfig.GetClaimId() != firstConfig.GetClaimId() {
		t.Fatalf(
			"reconstructed claim ID = %q, want %q",
			replacementConfig.GetClaimId(),
			firstConfig.GetClaimId(),
		)
	}
	replacement := NewController(tb.Logger, tb.Bus, replacementConfig)
	process(replacement)
	if replacement.execRoutine.GetState() == nil {
		t.Fatal("replacement controller did not recover the runnable execution")
	}
	if got := targetFactory.constructed.Load(); got != 0 {
		t.Fatalf("target construction count before replacement start = %d, want 0", got)
	}

	replacement.busEngine.SetContext(ctx)
	replacement.execRoutine.SetContext(ctx, true)
	finalState, err := forge_execution.WaitExecutionComplete(
		ctx,
		tb.Logger.WithField("control-loop", "claim-reconstruction-wait-complete"),
		tb.WorldState,
		objKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := finalState.GetExecutionState(); got != forge_execution.State_ExecutionState_COMPLETE {
		t.Fatalf("execution state = %s, want COMPLETE", got)
	}
	if errText := finalState.GetResult().GetFailError(); errText != "" {
		t.Fatalf("execution failed: %s", errText)
	}
	if got := targetFactory.constructed.Load(); got != 1 {
		t.Fatalf("target construction count = %d, want 1", got)
	}
}

type countingFactory struct {
	inner       controller.Factory
	constructed atomic.Int64
}

func (f *countingFactory) GetConfigID() string {
	return f.inner.GetConfigID()
}

func (f *countingFactory) ConstructConfig() config.Config {
	return f.inner.ConstructConfig()
}

func (f *countingFactory) Construct(
	ctx context.Context,
	conf config.Config,
	opts controller.ConstructOpts,
) (controller.Controller, error) {
	f.constructed.Add(1)
	return f.inner.Construct(ctx, conf, opts)
}

func (f *countingFactory) GetVersion() controller.Version {
	return f.inner.GetVersion()
}

// _ is a type assertion
var _ controller.Factory = (*countingFactory)(nil)
