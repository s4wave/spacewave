package execution_controller

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_lib_kvtx "github.com/s4wave/spacewave/forge/lib/kvtx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// leaseHandler blocks its first invocation until canceled, as a claimant that
// is still working when it dies, and completes every later invocation.
type leaseHandler struct {
	invocations *atomic.Int32
	started     chan<- struct{}
}

// Execute runs the handler.
func (h *leaseHandler) Execute(ctx context.Context) error {
	if h.invocations.Add(1) != 1 {
		return nil
	}
	close(h.started)
	<-ctx.Done()
	return ctx.Err()
}

func TestPeerReclaimsExecutionAfterClaimantDies(t *testing.T) {
	// Start a World testbed with the target controller factories.
	ctx := t.Context()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	tb.StaticResolver.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	tb.StaticResolver.AddFactory(forge_lib_kvtx.NewFactory(tb.Bus))

	// Register a target whose first run blocks and whose later runs complete.
	const configID = "test/claim-lease-handler"
	var invocations atomic.Int32
	started := make(chan struct{})
	registry := space_exec.NewRegistry()
	registry.Register(configID, func(
		context.Context,
		*logrus.Entry,
		world.WorldState,
		forge_target.ExecControllerHandle,
		forge_target.InputMap,
		[]byte,
	) (space_exec.Handler, error) {
		return &leaseHandler{invocations: &invocations, started: started}, nil
	})
	for _, factory := range space_exec.BridgeFactories(registry) {
		tb.StaticResolver.AddFactory(factory)
	}

	// Create a pending Execution for the target.
	target := &forge_target.Target{
		Exec: &forge_target.Exec{
			Controller: &configset_proto.ControllerConfig{Id: configID, Rev: 1},
		},
	}
	peerID := tb.Volume.GetPeerID()
	objKey := "test/execution/claim-lease"
	_, err = forge_execution.CreateExecutionWithTarget(
		ctx,
		tb.WorldState,
		peerID,
		objKey,
		peerID,
		forge_target.NewValueSet(),
		target,
		nil,
		timestamp.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Run each controller as its own claimant with a short lease.
	const claimLease = "1s"
	newController := func(claimID string) *Controller {
		conf := NewConfig(tb.EngineID, objKey, peerID, &forge_target.InputWorld{EngineId: tb.EngineID})
		conf.ClaimId = claimID
		conf.ClaimLease = claimLease
		return NewController(tb.Logger, tb.Bus, conf)
	}
	execute := func(ctrl *Controller) (stop func()) {
		execCtx, execCancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			done <- ctrl.Execute(execCtx)
			close(done)
		}()
		return func() {
			execCancel()
			if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("controller %s: %v", ctrl.claimID, err)
			}
		}
	}

	// Let the first claimant start the target under epoch one.
	stopDead := execute(newController("dead-claimant"))
	defer stopDead()
	<-started

	// Read the claim the first claimant holds.
	obj, err := world.MustGetObject(ctx, tb.WorldState, objKey)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)
	claimed, claimedObj, err := forge_execution.LookupExecution(ctx, tb.WorldState, objKey)
	world.ReleaseObjectState(claimedObj)
	if err != nil {
		t.Fatal(err)
	}
	deadClaim := claimed.GetClaim()
	if deadClaim.GetClaimId() != "dead-claimant" || deadClaim.GetEpoch() != 1 {
		t.Fatalf("claim = %q/%d, want dead-claimant/1", deadClaim.GetClaimId(), deadClaim.GetEpoch())
	}

	// Start two live peers that observe the claim, then kill the claimant.
	stopPeerA := execute(newController("peer-a"))
	defer stopPeerA()
	stopPeerB := execute(newController("peer-b"))
	defer stopPeerB()
	stopDead()

	// Wait for a peer to reclaim and complete the Execution.
	finalState, err := forge_execution.WaitExecutionComplete(
		ctx,
		tb.Logger.WithField("control-loop", "claim-lease"),
		tb.WorldState,
		objKey,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !finalState.GetResult().IsSuccessful() {
		t.Fatalf("execution failed: %s", finalState.GetResult().GetFailError())
	}

	// Require exactly one reclaim, so both peers did not take the claim.
	finalClaim := finalState.GetClaim()
	if finalClaim.GetEpoch() != 2 {
		t.Fatalf("claim epoch = %d, want 2", finalClaim.GetEpoch())
	}
	if id := finalClaim.GetClaimId(); id != "peer-a" && id != "peer-b" {
		t.Fatalf("claim holder = %q, want a live peer", id)
	}
	if got := invocations.Load(); got != 2 {
		t.Fatalf("target invocations = %d, want 2", got)
	}

	// Require the dead claimant's late writes to fail the epoch fence.
	var staleErr *execution_tx.StaleClaimEpochError
	_, _, err = obj.ApplyObjectOp(ctx, execution_tx.NewTxComplete(forge_value.NewResultWithSuccess(), deadClaim), peerID)
	if !errors.As(err, &staleErr) {
		t.Fatalf("late completion error = %v, want StaleClaimEpochError", err)
	}
	_, _, err = obj.ApplyObjectOp(ctx, execution_tx.NewTxRenewClaim(deadClaim, time.Now().Add(time.Hour)), peerID)
	if !errors.As(err, &staleErr) {
		t.Fatalf("late renewal error = %v, want StaleClaimEpochError", err)
	}
}
