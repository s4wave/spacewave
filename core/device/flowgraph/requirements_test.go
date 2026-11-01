//go:build !js

package device_flowgraph

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	"github.com/aperturerobotics/starpc/srpc"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_runtime "github.com/s4wave/spacewave/forge/runtime"
	"github.com/s4wave/spacewave/net/crypto"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_nodetype "github.com/s4wave/spacewave/sdk/flowgraph/nodetype"
	"github.com/sirupsen/logrus"
)

// TestRequirementPlacement runs two Device reconcilers through selector placement,
// shared-ledger admission, a competing claim and atomic removal of the grant.
func TestRequirementPlacement(t *testing.T) {
	// Seed two Devices, their real Forge Workers and a standing Flowgraph.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx)
	engine := tb.Engine
	selfPrivate, selfPeer := newSessionPeer(t)
	otherPrivate, otherPeer := newSessionPeer(t)
	createWorkers(t, ctx, engine, "cluster/requirements", map[string]crypto.PubKey{"worker/self": selfPrivate.GetPublic(), "worker/other": otherPrivate.GetPublic()})
	commit(t, ctx, engine, func(tx world.Tx) error {
		for key, peerID := range map[string]string{testSelfKey: selfPeer, testOtherKey: otherPeer} {
			labels := map[string]string{}
			if key == testOtherKey {
				labels["gpu"] = "true"
			}
			obj, _, err := world.CreateWorldObject(ctx, tx, key, func(cursor *block.Cursor) error {
				cursor.SetBlock(&s4wave_device.Device{PeerId: peerID, Label: key, Labels: labels}, true)
				return nil
			})
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			if err := world_types.SetObjectType(ctx, tx, key, s4wave_device.DeviceTypeID); err != nil {
				return err
			}
		}
		if _, _, err := tx.ApplyWorldOp(ctx, &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: testFlowgraphKey, Name: "requirements"}, ""); err != nil {
			return err
		}
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"fw-self": forgeWorkerNode("worker/self"), "fw-other": forgeWorkerNode("worker/other")}, SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacement{"fw-self": devicePlacement(testSelfKey), "fw-other": devicePlacement(testOtherKey)}})
		return err
	})

	// Give each Worker a live ownership fence and observed capacity on Forge's ledger.
	admission := forge_runtime.NewWorldRuntimeAdmission(engine, nil, time.Minute, time.Hour)
	for worker, device := range map[string]string{"worker/self": testSelfKey, "worker/other": testOtherKey} {
		ref := forge_runtime.WorkerClaimRef{DeviceObjectKey: device, ClaimID: worker}
		capacity, err := admission.ClaimWorkerCapacity(ctx, worker, ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admission.ObserveWorker(ctx, worker, ref, capacity.OwnerEpoch, 2000, 4294967296, []string{"docker", "fuse"}); err != nil {
			t.Fatal(err)
		}
	}

	// Run the production reconciler loops with real node type lookups and World watches.
	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(directive_controller.NewController(ctx, le))
	releaseTypes, err := b.AddController(ctx, flowgraph_nodetype.NewController(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseTypes()

	// Inject one stale placement commit while both Devices watch the candidate.
	raceEngine := &claimRaceEngine{Engine: engine}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 2)
	reconcilers := make(map[string]*Reconciler, 2)
	for deviceKey, peerID := range map[string]string{testSelfKey: selfPeer, testOtherKey: otherPeer} {
		stateRoot := t.TempDir()
		if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{NodeTypeId: []string{s4wave_flowgraph.ForgeWorkerNodeTypeID, "fixture"}}); err != nil {
			t.Fatal(err)
		}
		policy, err := device_policy.NewPolicyStore(stateRoot)
		if err != nil {
			t.Fatal(err)
		}
		r := NewReconciler(le, b, raceEngine, tb.EngineID, policy, NewForgeWorkerWatch(), deviceKey, peerID, func() func() { return func() {} })
		reconcilers[deviceKey] = r
	}

	// One selector matches; oversized and unsupported requests remain waiting.
	updateRequirements(t, ctx, engine, &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{
		"selected":  requirementNode(250, "docker", map[string]string{"gpu": "true"}),
		"too-large": requirementNode(3000, "docker", nil),
		"backend":   requirementNode(250, "unsupported", nil),
	}})

	// Observe the unplaced snapshot through the same RPC watch a graph client uses.
	graphResource := s4wave_flowgraph.NewFlowgraphResource(tb.WorldState, engine, b, tb.EngineID, testFlowgraphKey, "")
	graphClient := s4wave_flowgraph.NewSRPCFlowgraphResourceServiceClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(graphResource.GetMux()))))
	watch, err := graphClient.WatchFlowgraph(ctx, &s4wave_flowgraph.WatchFlowgraphRequest{})
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	pending, err := watch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if pending.GetSnapshot().GetPlacements()["selected"] != nil {
		t.Fatal("node was placed before the Device reconcilers started")
	}

	// Start both Devices only after the client has observed the waiting node.
	for _, r := range reconcilers {
		go func() { done <- r.Run(runCtx) }()
	}
	defer func() { stop(); <-done; <-done }()

	// A placement-only commit must wake the graph client and advance its revision.
	claimed, err := watch.Recv()
	if err != nil {
		t.Fatal(err)
	}
	if claimed.GetSnapshot().GetPlacements()["selected"].GetObjectKey() != testOtherKey || claimed.GetSnapshot().GetRevision() <= pending.GetSnapshot().GetRevision() {
		t.Fatal("graph watch did not receive the claim revision")
	}
	requireCapacity(t, ctx, engine, 0, 250)

	// A placement grant outlives runtime leases and Worker restart cleanup.
	if _, err := admission.ExpireLeases(ctx, forge_runtime.WorkerClaimRef{DeviceObjectKey: testOtherKey, ClaimID: "worker/other"}, time.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	capacity, err := admission.LookupWorkerCapacityAdmission(ctx, "worker/other")
	if err != nil {
		t.Fatal(err)
	}
	if err := admission.StopWorkerReservations(ctx, "worker/other", forge_runtime.WorkerClaimRef{DeviceObjectKey: testOtherKey, ClaimID: "worker/other"}, capacity.OwnerEpoch); err != nil {
		t.Fatal(err)
	}
	requireCapacity(t, ctx, engine, 0, 250)

	// Move the same holder to the other Device, then replay the old Device's cleanup.
	commit(t, ctx, engine, func(tx world.Tx) error {
		// Swap eligibility without changing the node's resource request.
		for key, value := range map[string]string{testSelfKey: "true", testOtherKey: "false"} {
			device, err := world.LookupObjectBody[s4wave_device.Device](ctx, tx, key)
			if err != nil {
				return err
			}
			device.Labels = map[string]string{"gpu": value}
			if _, _, err := world.AccessWorldObject(ctx, tx, key, true, func(cursor *block.Cursor) error { cursor.SetBlock(device, true); return nil }); err != nil {
				return err
			}
		}

		// Unplace and release the old grant in one edit before the new claim.
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{RemovePlacementNodeIds: []string{"selected"}})
		return err
	})
	waitRequirementPlacement(t, ctx, engine, func(snapshot *s4wave_flowgraph.FlowgraphSnapshot) bool {
		return snapshot.GetPlacements()["selected"].GetObjectKey() == testSelfKey
	})
	if err := reconcilers[testOtherKey].releasePlacement(ctx, forge_runtime.BuildReservationObjectKey(s4wave_flowgraph.NodeCapacityHolderKey(testFlowgraphKey, "selected"))); err != nil {
		t.Fatal(err)
	}
	requireCapacity(t, ctx, engine, 250, 0)

	// Make both Devices eligible, remove the old grant, and publish one shared candidate.
	commit(t, ctx, engine, func(tx world.Tx) error {
		// Give both Devices the selector before publishing the competing node.
		for _, key := range []string{testSelfKey, testOtherKey} {
			device, err := world.LookupObjectBody[s4wave_device.Device](ctx, tx, key)
			if err != nil {
				return err
			}
			device.Labels = map[string]string{"gpu": "true"}
			if _, _, err := world.AccessWorldObject(ctx, tx, key, true, func(cursor *block.Cursor) error { cursor.SetBlock(device, true); return nil }); err != nil {
				return err
			}
		}

		// Replace the selected node with a fresh candidate both Devices will see.
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{RemoveNodeIds: []string{"selected"}, SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"race": requirementNode(500, "docker", map[string]string{"gpu": "true"})}})
		return err
	})
	snapshot := waitRequirementPlacement(t, ctx, engine, func(snapshot *s4wave_flowgraph.FlowgraphSnapshot) bool {
		return snapshot.GetPlacements()["race"] != nil
	})
	if snapshot.GetPlacements()["too-large"] != nil || snapshot.GetPlacements()["backend"] != nil {
		t.Fatal("ineligible node was placed")
	}
	winner := snapshot.GetPlacements()["race"].GetObjectKey()
	selfCPU, otherCPU := uint64(0), uint64(500)
	if winner == testSelfKey {
		selfCPU, otherCPU = 500, 0
	}
	requireCapacity(t, ctx, engine, selfCPU, otherCPU)
	if raceEngine.candidates.Load() < 2 {
		t.Fatal("claim did not retry its stale World transaction")
	}

	// Removal releases in the same write, before either reconciler observes it.
	commit(t, ctx, engine, func(tx world.Tx) error {
		if _, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{RemovePlacementNodeIds: []string{"race"}, RemoveNodeIds: []string{"race"}}); err != nil {
			return err
		}
		for _, key := range []string{"worker/self", "worker/other"} {
			capacity, err := forge_runtime.LookupWorkerCapacity(ctx, tx, key)
			if err != nil {
				return err
			}
			if capacity.MilliCPUReserved != 0 || capacity.MemoryBytesReserved != 0 {
				t.Fatalf("removed node retained reservation: %+v", capacity)
			}
		}
		return nil
	})
	requireCapacity(t, ctx, engine, 0, 0)
}

// requirementNode builds a fixture standing node with a resource request.
func requirementNode(cpu uint64, backend string, labels map[string]string) *s4wave_flowgraph.FlowgraphNode {
	return &s4wave_flowgraph.FlowgraphNode{TypeId: "fixture", Requirements: &s4wave_flowgraph.FlowgraphRequirements{MilliCpu: cpu, MemoryBytes: 1024, RuntimeBackend: backend, MatchLabels: labels}}
}

// updateRequirements commits authored nodes through the production edit path.
func updateRequirements(t *testing.T, ctx context.Context, engine world.Engine, request *s4wave_flowgraph.UpdateFlowgraphRequest) {
	t.Helper()
	commit(t, ctx, engine, func(tx world.Tx) error {
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, request)
		return err
	})
}

// waitRequirementPlacement waits on the graph object revision for the asserted snapshot.
func waitRequirementPlacement(t *testing.T, ctx context.Context, engine world.Engine, matches func(*s4wave_flowgraph.FlowgraphSnapshot) bool) *s4wave_flowgraph.FlowgraphSnapshot {
	t.Helper()
	for {
		// Read the graph revision that the object watch must advance beyond.
		var snapshot *s4wave_flowgraph.FlowgraphSnapshot
		err := world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
			var err error
			snapshot, err = s4wave_flowgraph.ReadFlowgraph(ctx, ws, testFlowgraphKey)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if matches(snapshot) {
			return snapshot
		}
		if _, err := engine.WaitObjectRev(ctx, testFlowgraphKey, snapshot.GetRevision()+1, false); err != nil {
			t.Fatal(err)
		}
	}
}

// requireCapacity checks both Worker debits and the live placement grant count.
func requireCapacity(t *testing.T, ctx context.Context, engine world.Engine, selfCPU, otherCPU uint64) {
	t.Helper()
	err := world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
		// Read both capacities and the reservation index at the same committed revision.
		for key, want := range map[string]uint64{"worker/self": selfCPU, "worker/other": otherCPU} {
			capacity, err := forge_runtime.LookupWorkerCapacity(ctx, ws, key)
			if err != nil {
				return err
			}
			wantMemory := uint64(0)
			if want != 0 {
				wantMemory = 1024
			}
			if capacity.MilliCPUReserved != want || capacity.MemoryBytesReserved != wantMemory {
				t.Fatalf("%s capacity = %+v, want CPU %d, memory %d", key, capacity, want, wantMemory)
			}
		}
		grants, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("", forge_runtime.PlacementReservationPredicate, "", ""), 0)
		if err != nil {
			return err
		}
		wantGrants := 0
		if selfCPU+otherCPU != 0 {
			wantGrants = 1
		}
		if len(grants) != wantGrants {
			t.Fatalf("live placement grants = %d, want %d", len(grants), wantGrants)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
