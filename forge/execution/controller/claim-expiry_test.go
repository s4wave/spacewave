package execution_controller

import (
	"context"
	"sync/atomic"
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

// TestRestartedControllerReclaimsLapsedClaim restarts a controller over a claim
// it derives the same id for and requires it to replace that claim.
func TestRestartedControllerReclaimsLapsedClaim(t *testing.T) {
	for _, tc := range []struct {
		// name identifies the Execution state and lease position.
		name string
		// leaseOffset places the seeded lease expiry relative to the restart.
		leaseOffset time.Duration
		// cancel commits durable cancellation before the restart.
		cancel bool
	}{
		{name: "running with expired lease", leaseOffset: -time.Minute},
		{name: "canceling with expired lease", leaseOffset: -time.Minute, cancel: true},
		{name: "running inside the fence window", leaseOffset: time.Second},
	} {
		// Restart the controller through the production factory and World.
		t.Run(tc.name, func(t *testing.T) {
			// Start World services and register the Execution transactions.
			ctx := t.Context()
			tb, err := world_testbed.Default(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(tb.Release)
			release, err := tb.Bus.AddController(ctx, world.NewLookupOpController("claim-expiry-test-ops", tb.EngineID, execution_tx.LookupWorldOp), nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(release)

			// Count target starts through the production bridge.
			const configID = "test/lapsed-claim-target"
			var runs atomic.Int32
			registry := space_exec.NewRegistry()
			registry.Register(configID, func(context.Context, *logrus.Entry, world.WorldState, forge_target.ExecControllerHandle, forge_target.InputMap, []byte) (space_exec.Handler, error) {
				return &claimCountingHandler{invocations: &runs}, nil
			})
			for _, factory := range space_exec.BridgeFactories(registry) {
				tb.StaticResolver.AddFactory(factory)
			}
			tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

			// Create the Execution that a previous controller run claimed.
			peerID := tb.Volume.GetPeerID()
			const key = "test/lapsed-claim-execution"
			_, err = forge_execution.CreateExecutionWithTarget(ctx, tb.WorldState, peerID, key, peerID, forge_target.NewValueSet(), &forge_target.Target{
				Exec: &forge_target.Exec{Controller: &configset_proto.ControllerConfig{Id: configID, Rev: 1}},
			}, nil, timestamp.Now())
			if err != nil {
				t.Fatal(err)
			}

			// Seed the claim that the restarted controller derives the id of.
			conf := NewConfig(tb.EngineID, key, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
			obj, err := world.MustGetObject(ctx, tb.WorldState, key)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { world.ReleaseObjectState(obj) })
			lapsed := execution_tx.NewTxStart(peerID, time.Now().Add(tc.leaseOffset), conf.GetClaimId())
			if _, _, err := obj.ApplyObjectOp(ctx, lapsed, peerID); err != nil {
				t.Fatal(err)
			}

			// Commit durable cancellation under the seeded claim when requested.
			if tc.cancel {
				if _, _, err := obj.ApplyObjectOp(ctx, execution_tx.NewTxCancel(), peerID); err != nil {
					t.Fatal(err)
				}
			}

			// Restart the controller, which derives the id of the lapsed claim.
			_, ref, err := StartControllerWithConfig(ctx, tb.Bus, conf)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(ref.Release)

			// Wait for the restarted controller to settle the Execution.
			waitCtx, stop := context.WithTimeout(ctx, 10*time.Second)
			defer stop()
			state, err := forge_execution.WaitExecutionComplete(waitCtx, tb.Logger, tb.WorldState, key)
			if err != nil {
				t.Fatal(err)
			}

			// Require the replacement claim to carry the same id under a new epoch.
			if claim := state.GetClaim(); claim.GetClaimId() != conf.GetClaimId() || claim.GetEpoch() != 2 {
				t.Fatalf("settled claim = %v, want id %q at epoch 2", claim, conf.GetClaimId())
			}

			// Require a canceled Execution to finish canceled and a running one to succeed.
			if got := state.GetResult().GetCanceled(); got != tc.cancel {
				t.Fatalf("execution canceled = %v, want %v", got, tc.cancel)
			}
			if got := state.GetResult().IsSuccessful(); got == tc.cancel {
				t.Fatalf("execution result = %v, want canceled %v", state.GetResult(), tc.cancel)
			}

			// Require the target to start once for a running Execution and never for a canceled one.
			wantRuns := int32(1)
			if tc.cancel {
				wantRuns = 0
			}
			if got := runs.Load(); got != wantRuns {
				t.Fatalf("target invocations = %d, want %d", got, wantRuns)
			}
		})
	}
}
