package execution_controller

import (
	"context"
	"sync/atomic"
	"testing"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	"github.com/sirupsen/logrus"
)

// TestExecutionRetriesPluginInterruption distinguishes lost transport from a result.
func TestExecutionRetriesPluginInterruption(t *testing.T) {
	for _, tc := range []struct {
		// name identifies the typed transport or terminal handler outcome.
		name string
		// err is the first invocation's outcome.
		err error
		// retry requires a second invocation under the original claim.
		retry bool
		// cancel requests durable cancellation before the interrupted stream returns.
		cancel bool
	}{
		{name: "stream reset", err: srpc.ErrReset, retry: true},
		{name: "plugin stream closed", err: srpc.ErrClosedBeforeCompletion, retry: true},
		{name: "canceling stream reset", err: srpc.ErrReset, cancel: true},
		{name: "ordinary handler error", err: errors.New("handler failed")},
		{name: "handler cancellation", err: context.Canceled},
		{name: "reset message without typed signal", err: errors.New("stream reset")},
	} {
		// Exercise each first outcome through the real Execution controller and World.
		t.Run(tc.name, func(t *testing.T) {
			// Start World services and register the Execution transactions.
			ctx := t.Context()
			tb, err := world_testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)
			release, err := tb.Bus.AddController(ctx, world.NewLookupOpController("interruption-test-ops", tb.EngineID, execution_tx.LookupWorldOp), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)

			// Retain the invocation count and the second run's result gate.
			const configID = "test/interrupted-target"
			var runs atomic.Int32
			resumed := make(chan uint64, 1)
			finish := make(chan struct{})

			// Deliver durable cancellation before the first reset when requested.
			var onFirst func(context.Context) error
			if tc.cancel {
				onFirst = func(ctx context.Context) error {
					// Commit cancellation and await the controller's drain signal.
					obj, err := world.MustGetObject(ctx, tb.WorldState, "test/interrupted-execution")
					if err != nil {
						return err
					}
					defer world.ReleaseObjectState(obj)
					if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxCancel(), tb.Volume.GetPeerID()); err != nil {
						return err
					}
					<-ctx.Done()
					return nil
				}
			}

			// Reconstruct a handler on each target run through the production bridge.
			registry := space_exec.NewRegistry()
			registry.Register(configID, func(_ context.Context, _ *logrus.Entry, _ world.WorldState, handle forge_target.ExecControllerHandle, _ forge_target.InputMap, _ []byte) (space_exec.Handler, error) {
				return &interruptionHandler{firstErr: tc.err, onFirst: onFirst, runs: &runs, handle: handle, resumed: resumed, finish: finish}, nil
			})
			for _, factory := range space_exec.BridgeFactories(registry) {
				tb.StaticResolver.AddFactory(factory)
			}
			tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

			// Create a pending Execution and retain demand for its controller.
			peerID := tb.Volume.GetPeerID()
			const key = "test/interrupted-execution"
			_, err = forge_execution.CreateExecutionWithTarget(ctx, tb.WorldState, peerID, key, peerID, forge_target.NewValueSet(), &forge_target.Target{
				Exec: &forge_target.Exec{Controller: &configset_proto.ControllerConfig{Id: configID, Rev: 1}},
			}, nil, timestamp.Now())
			if err != nil {
				t.Fatal(err)
			}
			conf := NewConfig(tb.EngineID, key, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
			_, ref, err := StartControllerWithConfig(ctx, tb.Bus, conf)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(ref.Release)

			// An interrupted invocation keeps its active claim and has no failed result.
			if tc.retry {
				epoch := <-resumed
				state, obj, err := forge_execution.LookupExecution(ctx, tb.WorldState, key)
				world.ReleaseObjectState(obj)
				if err != nil {
					t.Fatal(err)
				}
				if state.GetExecutionState() != forge_execution.State_ExecutionState_RUNNING || state.GetResult() != nil {
					t.Fatalf("interrupted execution = %s, result %v", state.GetExecutionState(), state.GetResult())
				}
				if epoch != 1 || state.GetClaim().GetEpoch() != epoch || state.GetClaim().GetClaimId() != conf.GetClaimId() {
					t.Fatalf("retry epoch = %d, claim = %v", epoch, state.GetClaim())
				}
				close(finish)
			}

			// Require retry success or the first handler's terminal failure.
			state, err := forge_execution.WaitExecutionComplete(ctx, tb.Logger, tb.WorldState, key)
			if err != nil {
				t.Fatal(err)
			}
			wantRuns := int32(1)
			if tc.retry {
				wantRuns = 2
			}
			if got := runs.Load(); got != wantRuns {
				t.Fatalf("handler invocations = %d, want %d", got, wantRuns)
			}
			if state.GetResult().IsSuccessful() != tc.retry {
				t.Fatalf("execution result = %v, want success %v", state.GetResult(), tc.retry)
			}
			if state.GetResult().GetCanceled() != tc.cancel {
				t.Fatalf("execution canceled = %v, want %v", state.GetResult().GetCanceled(), tc.cancel)
			}
			if !tc.retry && !tc.cancel && state.GetResult().GetFailError() != errors.Wrap(tc.err, "receive plugin execution stream").Error() {
				t.Fatalf("handler failure = %q", state.GetResult().GetFailError())
			}
		})
	}
}
