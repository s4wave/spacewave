package execution_controller

import (
	"context"
	"testing"
	"time"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// TestCancellationInterruptsSetupAndDrainsTarget exercises durable cancellation
// before reconstruction and while a target is running.
func TestCancellationInterruptsSetupAndDrainsTarget(t *testing.T) {
	for _, running := range []bool{false, true} {
		// Select the cancellation scenario for the target's running state.
		name := "unavailable target after restart"
		if running {
			name = "running target drains before completion"
		}

		// Exercise durable cancellation before setup or during target execution.
		t.Run(name, func(t *testing.T) {
			// Use real World operations and an optional gated target handler.
			ctx := t.Context()
			tb, err := world_testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)

			// Gate the target's start, cancellation and drain when it is available.
			handler := &cancelDrainHandler{
				started:  make(chan struct{}),
				canceled: make(chan struct{}),
				drain:    make(chan struct{}),
			}
			const configID = "test/cancel-target"
			if running {
				registry := space_exec.NewRegistry()
				registry.Register(configID, func(context.Context, *logrus.Entry, world.WorldState, forge_target.ExecControllerHandle, forge_target.InputMap, []byte) (space_exec.Handler, error) {
					return handler, nil
				})
				for _, factory := range space_exec.BridgeFactories(registry) {
					tb.StaticResolver.AddFactory(factory)
				}
			}

			// Register the World operations that claim and cancel the Execution.
			release, err := tb.Bus.AddController(ctx, world.NewLookupOpController("cancel-test-ops", tb.EngineID, execution_tx.LookupWorldOp), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)

			// Create an Execution for the optional target controller.
			peerID := tb.Volume.GetPeerID()
			const key = "test/cancel-execution"
			_, err = forge_execution.CreateExecutionWithTarget(ctx, tb.WorldState, peerID, key, peerID, forge_target.NewValueSet(), &forge_target.Target{
				Exec: &forge_target.Exec{Controller: &configset_proto.ControllerConfig{Id: configID, Rev: 1}},
			}, nil, timestamp.Now())
			if err != nil {
				t.Fatal(err)
			}

			// Retain the Execution object while applying its durable claim.
			obj, err := world.MustGetObject(ctx, tb.WorldState, key)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { world.ReleaseObjectState(obj) })
			conf := NewConfig(tb.EngineID, key, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
			if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxStart(peerID, time.Now().Add(time.Hour), conf.GetClaimId()), peerID); err != nil {
				t.Fatal(err)
			}

			// Prepare the claimed controller and a snapshot reconciliation helper.
			ctrl := NewController(tb.Logger, tb.Bus, conf)
			t.Cleanup(func() { _ = ctrl.Close() })
			process := func() {
				// Read the Execution root for the next controller reconciliation.
				t.Helper()
				ref, rev, err := obj.GetRootRef(ctx)
				if err != nil {
					t.Fatal(err)
				}

				// Apply the observed Execution state to the target routine.
				if _, err := ctrl.ProcessState(ctx, tb.Logger, tb.WorldState, obj, ref, rev); err != nil {
					t.Fatal(err)
				}
			}

			// Start the available target and wait until its handler is running.
			ctrl.busEngine.SetContext(ctx)
			if running {
				ctrl.execRoutine.SetContext(ctx, true)
				process()
				<-handler.started
			}

			// Cancellation is durable before the replacement routine starts.
			if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxCancel(), peerID); err != nil {
				t.Fatal(err)
			}
			process()

			// Require a late listener to observe the durable cancellation signal.
			select {
			case <-ctrl.CancelWaitCh():
			default:
				t.Fatal("late listener missed durable cancellation")
			}

			// Verify the running target retains the Execution until it drains.
			if running {
				// Observe the target's cancellation and read its durable Execution state.
				<-handler.canceled
				state, stateObj, err := forge_execution.LookupExecution(ctx, tb.WorldState, key)
				world.ReleaseObjectState(stateObj)
				if err != nil {
					t.Fatal(err)
				}

				// Require the Execution to remain canceling while the target holds work.
				if state.GetExecutionState() != forge_execution.State_ExecutionState_CANCELING {
					t.Fatal("execution completed before target drained")
				}

				// Release the target's remaining work so cancellation can settle.
				close(handler.drain)
			} else {
				ctrl.execRoutine.SetContext(ctx, true)
			}

			// Completion retains the claim and records cancellation, never success.
			state, err := forge_execution.WaitExecutionComplete(ctx, tb.Logger, tb.WorldState, key)
			if err != nil {
				t.Fatal(err)
			}

			// Require cancellation to settle as a canceled Execution result.
			if state.GetResult().IsSuccessful() || !state.GetResult().GetCanceled() {
				t.Fatalf("expected canceled result: %v", state.GetResult())
			}
		})
	}
}

// TestCancellationCommittedAsTargetFinishes settles an execution whose target
// returns success after a durable cancel the controller has not yet observed.
func TestCancellationCommittedAsTargetFinishes(t *testing.T) {
	// Start a World testbed for cancellation during successful target completion.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)

	// Register a target handler that returns success when released.
	handler := &finishHandler{started: make(chan struct{}), finish: make(chan struct{})}
	const configID = "test/finish-target"
	registry := space_exec.NewRegistry()
	registry.Register(configID, func(context.Context, *logrus.Entry, world.WorldState, forge_target.ExecControllerHandle, forge_target.InputMap, []byte) (space_exec.Handler, error) {
		return handler, nil
	})
	for _, factory := range space_exec.BridgeFactories(registry) {
		tb.StaticResolver.AddFactory(factory)
	}

	// Register the World operations that claim and cancel the Execution.
	release, err := tb.Bus.AddController(ctx, world.NewLookupOpController("cancel-test-ops", tb.EngineID, execution_tx.LookupWorldOp), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Start the execution under a claim and wait for the target to run.
	peerID := tb.Volume.GetPeerID()
	const key = "test/finish-execution"
	_, err = forge_execution.CreateExecutionWithTarget(ctx, tb.WorldState, peerID, key, peerID, forge_target.NewValueSet(), &forge_target.Target{
		Exec: &forge_target.Exec{Controller: &configset_proto.ControllerConfig{Id: configID, Rev: 1}},
	}, nil, timestamp.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Retain the Execution object and commit the controller's claim.
	obj, err := world.MustGetObject(ctx, tb.WorldState, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { world.ReleaseObjectState(obj) })
	conf := NewConfig(tb.EngineID, key, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
	if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxStart(peerID, time.Now().Add(time.Hour), conf.GetClaimId()), peerID); err != nil {
		t.Fatal(err)
	}

	// Start the claimed target and wait for its handler to begin.
	ctrl := NewController(tb.Logger, tb.Bus, conf)
	t.Cleanup(func() { _ = ctrl.Close() })
	ctrl.busEngine.SetContext(ctx)
	ctrl.execRoutine.SetContext(ctx, true)
	ref, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.ProcessState(ctx, tb.Logger, tb.WorldState, obj, ref, rev); err != nil {
		t.Fatal(err)
	}
	<-handler.started

	// Commit the cancel without delivering it to the controller, then finish.
	if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxCancel(), peerID); err != nil {
		t.Fatal(err)
	}
	close(handler.finish)

	// Require durable cancellation to override the target's successful return.
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	state, err := forge_execution.WaitExecutionComplete(waitCtx, tb.Logger, tb.WorldState, key)
	if err != nil {
		t.Fatal(err)
	}
	if !state.GetResult().GetCanceled() {
		t.Fatalf("expected canceled result: %v", state.GetResult())
	}
}
