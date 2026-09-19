package execution_controller

import (
	"context"
	"testing"

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
		name := "unavailable target after restart"
		if running {
			name = "running target drains before completion"
		}
		t.Run(name, func(t *testing.T) {
			// Use real World operations and an optional gated target handler.
			ctx := t.Context()
			tb, err := world_testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)
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
			release, err := tb.Bus.AddController(ctx, world.NewLookupOpController("cancel-test-ops", tb.EngineID, execution_tx.LookupWorldOp), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)
			peerID := tb.Volume.GetPeerID()
			const key = "test/cancel-execution"
			_, err = forge_execution.CreateExecutionWithTarget(ctx, tb.WorldState, peerID, key, peerID, forge_target.NewValueSet(), &forge_target.Target{
				Exec: &forge_target.Exec{Controller: &configset_proto.ControllerConfig{Id: configID, Rev: 1}},
			}, timestamp.Now())
			if err != nil {
				t.Fatal(err)
			}
			obj, err := world.MustGetObject(ctx, tb.WorldState, key)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { world.ReleaseObjectState(obj) })
			conf := NewConfig(tb.EngineID, key, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
			if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxStart(peerID, conf.GetClaimId()), peerID); err != nil {
				t.Fatal(err)
			}
			ctrl := NewController(tb.Logger, tb.Bus, conf)
			t.Cleanup(func() { _ = ctrl.Close() })
			process := func() {
				t.Helper()
				ref, rev, err := obj.GetRootRef(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := ctrl.ProcessState(ctx, tb.Logger, tb.WorldState, obj, ref, rev); err != nil {
					t.Fatal(err)
				}
			}
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
			select {
			case <-ctrl.CancelWaitCh():
			default:
				t.Fatal("late listener missed durable cancellation")
			}
			if running {
				<-handler.canceled
				state, stateObj, err := forge_execution.LookupExecution(ctx, tb.WorldState, key)
				world.ReleaseObjectState(stateObj)
				if err != nil {
					t.Fatal(err)
				}
				if state.GetExecutionState() != forge_execution.State_ExecutionState_CANCELING {
					t.Fatal("execution completed before target drained")
				}
				close(handler.drain)
			} else {
				ctrl.execRoutine.SetContext(ctx, true)
			}

			// Completion retains the claim and records cancellation, never success.
			state, err := forge_execution.WaitExecutionComplete(ctx, tb.Logger, tb.WorldState, key)
			if err != nil {
				t.Fatal(err)
			}
			if state.GetResult().IsSuccessful() || !state.GetResult().GetCanceled() {
				t.Fatalf("expected canceled result: %v", state.GetResult())
			}
		})
	}
}

// cancelDrainHandler separates observing cancellation from draining work.
type cancelDrainHandler struct {
	started  chan struct{}
	canceled chan struct{}
	drain    chan struct{}
}

// Execute waits for cancellation and retains execution custody until drained.
func (h *cancelDrainHandler) Execute(ctx context.Context) error {
	close(h.started)
	<-ctx.Done()
	close(h.canceled)
	<-h.drain
	return ctx.Err()
}

// _ is a type assertion.
var _ space_exec.Handler = (*cancelDrainHandler)(nil)
