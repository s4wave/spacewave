package execution_controller_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	space_exec "github.com/s4wave/spacewave/core/forge/exec"
	"github.com/s4wave/spacewave/db/world"
	forge_core "github.com/s4wave/spacewave/forge/core"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
	forge_target "github.com/s4wave/spacewave/forge/target"
	forge_testbed "github.com/s4wave/spacewave/forge/testbed"
	forge_value "github.com/s4wave/spacewave/forge/value"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	worker_controller "github.com/s4wave/spacewave/forge/worker/controller"
	forge_world "github.com/s4wave/spacewave/forge/world"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	peer_controller "github.com/s4wave/spacewave/net/peer/controller"
	"github.com/sirupsen/logrus"
)

// TestPeerReclaimsExecutionAfterWorkerDies runs two real Workers on independent
// buses with distinct peers. Synctest injects time into timers and timestamps.
func TestPeerReclaimsExecutionAfterWorkerDies(t *testing.T) {
	synctest.Test(t, testPeerReclaimsExecutionAfterWorkerDies)
}

// testPeerReclaimsExecutionAfterWorkerDies verifies that only the surviving peer
// takes custody after production Worker demand ends.
func testPeerReclaimsExecutionAfterWorkerDies(t *testing.T) {
	// Start World and Forge factories under the injected clock.
	ctx := t.Context()
	tb, err := forge_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	op := world.NewLookupOpController("forge-lease-ops", tb.EngineID, forge_world.LookupWorldOp)
	stopOp, err := tb.Bus.AddController(ctx, op, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stopOp)

	// Mount the same authorized World in the second daemon's independent bus.
	secondBus, secondResolver, err := forge_core.NewCoreBus(ctx, tb.Logger)
	if err != nil {
		t.Fatal(err)
	}
	stopMount, err := secondBus.AddController(ctx, world.NewEngineController(tb.EngineID, tb.Engine), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stopMount)

	// Register a target that retains its first claimant until Worker cancellation.
	const configID = "test/claim-lease-handler"
	var invocations atomic.Int32
	started := make(chan struct{})
	drained := make(chan struct{})
	registry := space_exec.NewRegistry()
	registry.Register(configID, func(
		context.Context, *logrus.Entry, world.WorldState,
		forge_target.ExecControllerHandle, forge_target.InputMap, []byte,
	) (space_exec.Handler, error) {
		return &leaseHandler{invocations: &invocations, started: started, drained: drained}, nil
	})
	for _, factory := range space_exec.BridgeFactories(registry) {
		tb.StaticResolver.AddFactory(factory)
		secondResolver.AddFactory(factory)
	}

	// Enroll distinct peer identities and their Workers in the same World.
	peers := make([]peer.Peer, 0, 2)
	buses := []bus.Bus{tb.Bus, secondBus}
	for i, name := range []string{"claimant", "reclaimer"} {
		// Attach the Worker's peer to the controller bus.
		p, err := peer.NewPeer(nil)
		if err != nil {
			t.Fatal(err)
		}
		stopPeer, err := buses[i].AddController(ctx, peer_controller.NewController(tb.Logger, p), nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(stopPeer)

		// Link the Worker to its peer keypair.
		keypair, err := identity.NewKeypair(p.GetPubKey(), "", nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := forge_worker.CreateWorker(ctx, tb.WorldState, "workers/"+name, name, []*identity.Keypair{keypair}, tb.Volume.GetPeerID()); err != nil {
			t.Fatal(err)
		}
		peers = append(peers, p)
	}

	// Create a placed Execution and start its claimant's production Worker.
	const objKey = "exec/claim-lease"
	firstPeer := peers[0].GetPeerID()
	placement := &forge_worker.Placement{WorkerObjectKey: "workers/claimant", PeerId: firstPeer.String()}
	target := &forge_target.Target{Exec: &forge_target.Exec{
		Controller: &configset_proto.ControllerConfig{Id: configID, Rev: 1},
	}}
	_, err = forge_execution.CreateExecutionWithTarget(ctx, tb.WorldState, firstPeer, objKey, firstPeer,
		forge_target.NewValueSet(), target, placement, timestamp.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Retain the first Worker's demand until the test simulates its death.
	claimantCtx, stopClaimant := context.WithCancel(ctx)
	t.Cleanup(stopClaimant)
	_, claimantRef, err := worker_controller.StartControllerWithConfig(claimantCtx, tb.Bus,
		worker_controller.NewConfig(tb.EngineID, placement.GetWorkerObjectKey(), firstPeer, true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(claimantRef.Release)
	<-started

	// Retain the original epoch for late writes and its original expiry bound.
	claimed, obj, err := forge_execution.LookupExecution(ctx, tb.WorldState, objKey)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { world.ReleaseObjectState(obj) })
	deadClaim := claimed.GetClaim().CloneVT()
	if deadClaim.GetEpoch() != 1 {
		t.Fatalf("initial claim epoch = %d, want 1", deadClaim.GetEpoch())
	}

	// Give the second Worker standing demand to observe this shared Execution.
	secondPeer := peers[1].GetPeerID()
	_, _, err = identity_world.LinkObjectToKeypair(ctx, tb.WorldState, firstPeer, objKey, secondPeer, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, reclaimerRef, err := worker_controller.StartControllerWithConfig(ctx, secondBus,
		worker_controller.NewConfig(tb.EngineID, "workers/reclaimer", secondPeer, true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(reclaimerRef.Release)
	synctest.Wait()

	// Kill the first Worker and wait for its target to drain before expiry.
	stopClaimant()
	claimantRef.Release()
	<-drained
	synctest.Wait()
	if !time.Now().Before(deadClaim.GetLeaseExpiresAt().AsTime()) {
		t.Fatal("claimant did not stop before its lease deadline")
	}

	// Advance injected time to expiry by waiting for final settlement.
	finalState, err := forge_execution.WaitExecutionComplete(ctx, tb.Logger, tb.WorldState, objKey)
	if err != nil {
		t.Fatal(err)
	}
	synctest.Wait()
	if !finalState.GetResult().IsSuccessful() {
		t.Fatalf("execution failed: %s", finalState.GetResult().GetFailError())
	}
	if got := invocations.Load(); got != 2 {
		t.Fatalf("target invocations = %d, want 2", got)
	}
	if finalState.GetClaim().GetEpoch() != 2 || finalState.GetPeerId() != secondPeer.String() {
		t.Fatalf("reclaimed epoch/peer = %d/%s", finalState.GetClaim().GetEpoch(), finalState.GetPeerId())
	}

	// Require the surviving Worker's placement and sole Worker graph edge.
	wantPlacement := &forge_worker.Placement{WorkerObjectKey: "workers/reclaimer", PeerId: secondPeer.String()}
	if !finalState.GetPlacement().EqualVT(wantPlacement) {
		t.Fatalf("placement = %v, want %v", finalState.GetPlacement(), wantPlacement)
	}
	quads, err := tb.WorldState.LookupGraphQuads(ctx, forge_execution.NewExecutionToWorkerQuad(objKey, ""), 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(quads) != 1 || quads[0].GetObj() != forge_execution.NewExecutionToWorkerQuad(objKey, "workers/reclaimer").GetObj() {
		t.Fatalf("Worker edges = %v", quads)
	}

	// Reject a competing reclaim and the dead claimant's late fenced writes.
	now := time.Now()
	_, _, err = tb.WorldState.ApplyWorldOp(ctx, execution_tx.NewTxReclaim(objKey, firstPeer, "late-reclaimer", 1, now,
		now.Add(forge_execution.DefaultClaimLease), placement), firstPeer)
	if err == nil {
		t.Fatal("a second reclaim succeeded")
	}
	for _, tx := range []*execution_tx.Tx{
		execution_tx.NewTxComplete(forge_value.NewResultWithSuccess(), deadClaim),
		execution_tx.NewTxRenewClaim(deadClaim, now.Add(forge_execution.DefaultClaimLease)),
	} {
		_, _, err := obj.ApplyObjectOp(ctx, tx, firstPeer)
		if _, ok := errors.AsType[*execution_tx.StaleClaimEpochError](err); !ok {
			t.Fatalf("late %s error = %v, want StaleClaimEpochError", tx.GetTxType(), err)
		}
	}
}
