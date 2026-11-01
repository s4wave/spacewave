//go:build !js

package device_flowgraph

import (
	"context"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/bus/inmem"
	"github.com/aperturerobotics/controllerbus/controller"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	terminal_remoteshell "github.com/s4wave/spacewave/core/terminal/remoteshell"
	"github.com/s4wave/spacewave/core/transport"
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

// TestRequirementReplicas runs three Devices through count changes and lease replacement.
// Each claimed copy starts a real remote-shell controller on its Session bus.
func TestRequirementReplicas(t *testing.T) {
	// Seed three matching Devices with a real Worker and Session identity each.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	tb := world_testbed.MustDefault(t, ctx, world_testbed.WithChangelog())
	engine := tb.Engine
	clock := &leaseClock{now: time.Now(), timers: make(map[*leaseTimer]struct{})}
	devices := []string{testSelfKey, testOtherKey, "device/third"}
	workers := []string{"worker/self", "worker/other", "worker/third"}
	keys := make([]crypto.PrivKey, len(devices))
	peers := make([]string, len(devices))
	publics := make(map[string]crypto.PubKey, len(devices))
	for i := range devices {
		keys[i], peers[i] = newSessionPeer(t)
		publics[workers[i]] = keys[i].GetPublic()
	}
	createWorkers(t, ctx, engine, "cluster/replicas", publics)
	commit(t, ctx, engine, func(tx world.Tx) error {
		// Create each typed Device and its explicitly placed Worker declaration.
		nodes := make(map[string]*s4wave_flowgraph.FlowgraphNode, len(devices))
		placements := make(map[string]*s4wave_flowgraph.FlowgraphPlacements, len(devices))
		for i, key := range devices {
			obj, _, err := world.CreateWorldObject(ctx, tx, key, func(cursor *block.Cursor) error {
				cursor.SetBlock(&s4wave_device.Device{PeerId: peers[i], Label: key, Labels: map[string]string{"pool": "replicas"}}, true)
				return nil
			})
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			if err := world_types.SetObjectType(ctx, tx, key, s4wave_device.DeviceTypeID); err != nil {
				return err
			}
			nodes["fw-"+strconv.Itoa(i)] = forgeWorkerNode(workers[i])
			placements["fw-"+strconv.Itoa(i)] = &s4wave_flowgraph.FlowgraphPlacements{Copies: []*s4wave_flowgraph.FlowgraphPlacement{devicePlacement(key)}}
		}

		// Start with two desired copies and no placement for the shell.
		if _, _, err := tx.ApplyWorldOp(ctx, &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: testFlowgraphKey, Name: "replicas"}, ""); err != nil {
			return err
		}
		node := requirementNode(250, "docker", map[string]string{"pool": "replicas"})
		node.TypeId = s4wave_flowgraph.RemoteShellNodeTypeID
		node.Requirements.Replicas = 2
		nodes["race"] = node
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: nodes, SetPlacements: placements})
		return err
	})

	// Share Forge's ledger and frozen clock for ownership and placement admission.
	admission := forge_runtime.NewWorldRuntimeAdmission(engine, nil, time.Minute, time.Hour)
	admission.SetTimeNow(clock.Now)
	for i, worker := range workers {
		ref := forge_runtime.WorkerClaimRef{DeviceObjectKey: devices[i], ClaimID: worker}
		capacity, err := admission.ClaimWorkerCapacity(ctx, worker, ref)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := admission.ObserveWorker(ctx, worker, ref, capacity.OwnerEpoch, 2000, 4294967296, []string{"docker"}); err != nil {
			t.Fatal(err)
		}
	}

	// Resolve built-in node types and run real remote-shell controllers on each Session.
	le := logrus.NewEntry(logrus.New())
	b := inmem.NewBus(directive_controller.NewController(ctx, le))
	release, err := b.AddController(ctx, flowgraph_nodetype.NewController(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	raceEngine := &claimRaceEngine{Engine: engine}
	reconcilers := make([]*Reconciler, len(devices))
	for i := range devices {
		// Each policy permits its Worker declaration and the replicated shell.
		root := t.TempDir()
		if err := device_policy.WriteFile(root, &device_policy.DevicePolicy{NodeTypeId: []string{s4wave_flowgraph.ForgeWorkerNodeTypeID, s4wave_flowgraph.RemoteShellNodeTypeID}}); err != nil {
			t.Fatal(err)
		}
		policy, err := device_policy.NewPolicyStore(root)
		if err != nil {
			t.Fatal(err)
		}
		r := NewReconciler(le, b, raceEngine, tb.EngineID, policy, NewForgeWorkerWatch(), devices[i], peers[i], func() func() { return func() {} })
		r.now, r.atFunc = clock.Now, clock.AtFunc
		reconcilers[i] = r

		// The Session hosts the shell controller and is released with the test.
		st, err := transport.NewSessionTransport(le, b, keys[i], "", "", transport.WithChildFactories(func(b bus.Bus) controller.Factory { return terminal_remoteshell.NewFactory(b) }))
		if err != nil {
			t.Fatal(err)
		}
		sessionDone := make(chan error, 1)
		go func() { sessionDone <- st.Execute(ctx) }()
		t.Cleanup(func() { cancel(); <-sessionDone })
		if err := st.AwaitReady(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// The first holder's claim precedes the race between the other two Devices.
	start := func(r *Reconciler) {
		done := make(chan error, 1)
		go func() { done <- r.Run(ctx) }()
		t.Cleanup(func() {
			cancel()
			if err := <-done; err != nil {
				t.Error(err)
			}
		})
	}
	start(reconcilers[0])
	waitRequirementPlacement(t, ctx, engine, func(snapshot *s4wave_flowgraph.FlowgraphSnapshot) bool {
		return len(snapshot.GetPlacements()["race"].GetCopies()) == 1
	})
	start(reconcilers[1])
	start(reconcilers[2])
	initial := waitRequirementPlacement(t, ctx, engine, func(snapshot *s4wave_flowgraph.FlowgraphSnapshot) bool {
		return len(snapshot.GetPlacements()["race"].GetCopies()) == 2
	})
	requireReplicaLedger(t, ctx, engine, devices, workers, initial.GetPlacements()["race"])
	if raceEngine.candidates.Load() < 2 {
		t.Fatal("replica claim did not retry its stale transaction")
	}
	for _, copy := range initial.GetPlacements()["race"].GetCopies() {
		waitReplicaState(t, ctx, engine, copy.GetObjectKey(), true)
	}

	// Reducing only the count preserves the oldest copy and releases the newer grant.
	node := initial.GetState().GetNodes()["race"].CloneVT()
	node.Requirements.Replicas = 1
	updateRequirements(t, ctx, engine, &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"race": node}})
	reduced := waitRequirementPlacement(t, ctx, engine, func(snapshot *s4wave_flowgraph.FlowgraphSnapshot) bool {
		return len(snapshot.GetPlacements()["race"].GetCopies()) == 1
	})
	if reduced.GetPlacements()["race"].First().GetObjectKey() != devices[0] {
		t.Fatal("count reduction released the oldest copy")
	}
	requireReplicaLedger(t, ctx, engine, devices, workers, reduced.GetPlacements()["race"])
	for _, copy := range initial.GetPlacements()["race"].GetCopies() {
		if copy.GetObjectKey() != devices[0] {
			waitReplicaState(t, ctx, engine, copy.GetObjectKey(), false)
		}
	}

	// Restore the second copy before expiring one holder while the third Device waits.
	node.Requirements.Replicas = 2
	updateRequirements(t, ctx, engine, &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"race": node}})
	restored := waitRequirementPlacement(t, ctx, engine, func(snapshot *s4wave_flowgraph.FlowgraphSnapshot) bool {
		return len(snapshot.GetPlacements()["race"].GetCopies()) == 2
	})
	for _, copy := range restored.GetPlacements()["race"].GetCopies() {
		waitReplicaState(t, ctx, engine, copy.GetObjectKey(), true)
	}
	expiredDevice := devices[0]
	survivor := ""
	standby := ""
	for _, device := range devices[1:] {
		if isPlacedOn(restored.GetPlacements()["race"], device) {
			survivor = device
			continue
		}
		standby = device
	}

	// Keep the other two Workers live, but stop renewing the first holder.
	admission.SetTimeNow(func() time.Time { return clock.Now().Add(time.Hour) })
	for i := 1; i < len(workers); i++ {
		if _, err := admission.ClaimWorkerCapacity(ctx, workers[i], forge_runtime.WorkerClaimRef{DeviceObjectKey: devices[i], ClaimID: workers[i]}); err != nil {
			t.Fatal(err)
		}
	}
	admission.SetTimeNow(clock.Now)
	clock.Advance(time.Hour + time.Nanosecond)
	replaced := waitRequirementPlacement(t, ctx, engine, func(snapshot *s4wave_flowgraph.FlowgraphSnapshot) bool {
		copies := snapshot.GetPlacements()["race"]
		return len(copies.GetCopies()) == 2 && !isPlacedOn(copies, expiredDevice) && isPlacedOn(copies, survivor) && isPlacedOn(copies, standby)
	})
	requireReplicaLedger(t, ctx, engine, devices, workers, replaced.GetPlacements()["race"])
	waitReplicaState(t, ctx, engine, standby, true)

	// Renewing the departed holder cannot resurrect its old edge or controller.
	if _, err := admission.ClaimWorkerCapacity(ctx, workers[0], forge_runtime.WorkerClaimRef{DeviceObjectKey: expiredDevice, ClaimID: workers[0]}); err != nil {
		t.Fatal(err)
	}
	waitReplicaState(t, ctx, engine, expiredDevice, false)
	requireReplicaLedger(t, ctx, engine, devices, workers, replaced.GetPlacements()["race"])

	// The edit history must replay the full surviving set after an atomic replacement.
	err = world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
		history, err := s4wave_flowgraph.ReadFlowgraphChanges(ctx, ws, testFlowgraphKey, 0)
		if err != nil {
			return err
		}
		for _, change := range history.GetChanges() {
			if copies := change.GetEdit().GetSetPlacements()["race"]; len(copies.GetCopies()) == 2 && isPlacedOn(copies, standby) && isPlacedOn(copies, survivor) {
				return nil
			}
		}
		t.Fatal("history lost the surviving replica during replacement")
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// requireReplicaLedger checks every edge's live grant and both resource debits together.
func requireReplicaLedger(t *testing.T, ctx context.Context, engine world.Engine, devices, workers []string, group *s4wave_flowgraph.FlowgraphPlacements) {
	t.Helper()
	err := world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
		// Compare the indexed live grants with the expected placement set.
		grants, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("", forge_runtime.PlacementReservationPredicate, "", ""), 0)
		if err != nil {
			return err
		}
		if len(grants) != len(group.GetCopies()) {
			t.Fatalf("live grants = %d, want %d", len(grants), len(group.GetCopies()))
		}
		for i, device := range devices {
			capacity, err := forge_runtime.LookupWorkerCapacity(ctx, ws, workers[i])
			if err != nil {
				return err
			}
			cpu, memory := uint64(0), uint64(0)
			if isPlacedOn(group, device) {
				cpu, memory = 250, 1024
			}
			if capacity.MilliCPUReserved != cpu || capacity.MemoryBytesReserved != memory {
				t.Fatalf("%s debit = %d/%d, want %d/%d", device, capacity.MilliCPUReserved, capacity.MemoryBytesReserved, cpu, memory)
			}
			key := forge_runtime.BuildReservationObjectKey(s4wave_flowgraph.NodeCapacityHolderKey(testFlowgraphKey, "race", device))
			res, err := forge_runtime.LookupReservation(ctx, ws, key)
			if cpu != 0 {
				if err != nil {
					return err
				}
				if res.State.Terminal() {
					t.Fatal("placement has a terminal reservation")
				}
			}
			if cpu == 0 && err == nil && !res.State.Terminal() {
				t.Fatal("unplaced Device retains a live reservation")
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// waitReplicaState waits on the Device report for the applied or removed shell copy.
func waitReplicaState(t *testing.T, ctx context.Context, engine world.Engine, deviceKey string, active bool) {
	t.Helper()
	for {
		// Observe the same Device revision whose capabilities the assertion reads.
		var device *s4wave_device.Device
		var rev uint64
		err := world.ExecTransaction(ctx, engine, false, func(ctx context.Context, ws world.WorldState) error {
			obj, _, err := ws.GetObject(ctx, deviceKey)
			defer world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
			device, err = readDevice(ctx, obj)
			if err != nil {
				return err
			}
			_, rev, err = obj.GetRootRef(ctx)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		found := slices.ContainsFunc(device.GetCapabilities(), func(c *s4wave_device.DeviceCapability) bool {
			return c.GetKind() == s4wave_device.DeviceCapabilityKindRemoteShell && c.GetState() == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE
		})
		present := slices.ContainsFunc(device.GetCapabilities(), func(c *s4wave_device.DeviceCapability) bool {
			return c.GetKind() == s4wave_device.DeviceCapabilityKindRemoteShell
		})
		if active && found || !active && !present {
			return
		}
		if _, err := engine.WaitObjectRev(ctx, deviceKey, rev+1, false); err != nil {
			t.Fatalf("Device %s shell active=%v: %v", deviceKey, active, err)
		}
	}
}
