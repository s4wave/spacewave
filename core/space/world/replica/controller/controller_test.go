//go:build !tinygo

package space_world_replica_controller_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	device_flowgraph "github.com/s4wave/spacewave/core/device/flowgraph"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world_replica "github.com/s4wave/spacewave/core/space/world/replica"
	space_world_replica_controller "github.com/s4wave/spacewave/core/space/world/replica/controller"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_nodetype "github.com/s4wave/spacewave/sdk/flowgraph/nodetype"
	"github.com/s4wave/spacewave/testbed"
)

// TestReplicaMovesOwnershipOverAndBack places a replica pair in a Flowgraph
// on two accounts of one provider, each run by its Device's reconciler.
// Ownership moves to the desired owner after it proves its copy of the World,
// and moves back once the Flowgraph names the first node again.
func TestReplicaMovesOwnershipOverAndBack(t *testing.T) {
	// Bound the run.
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()

	// Two accounts on one provider, each running its Session transport with
	// the replica factory on its child bus.
	tb := startTestbed(ctx, t)
	a, aSession := setupAccount(ctx, t, tb)
	b, bSession := setupAccount(ctx, t, tb)
	aPeer, bPeer := aSession.GetPeerId().String(), bSession.GetPeerId().String()

	// A owns a Space holding one object, which B must copy before it owns.
	ref, err := a.CreateSharedObject(ctx, "replica-space", mustSpaceMeta(t), "", "")
	if err != nil {
		t.Fatal(err)
	}
	id := ref.GetProviderResourceRef().GetId()
	writeObject(ctx, t, tb.Bus, ref, "replica-check")

	// Moving to B leaves A no longer a participant, and B owning a copy of
	// the object. A leave keeps the departed copy listed.
	graph := startFlowgraph(ctx, t, tb, aPeer, bPeer)
	placePair(ctx, t, graph, id, "b")
	waitOwner(ctx, t, b, id, bPeer, aPeer)
	checkObject(ctx, t, tb.Bus, b, id, "replica-check")

	// Moving back needs a fresh invite, since A's leave gave up its grants.
	placePair(ctx, t, graph, id, "a")
	waitOwner(ctx, t, a, id, aPeer, bPeer)
	checkObject(ctx, t, tb.Bus, a, id, "replica-check")
}

// startTestbed starts a testbed running the local provider, whose Session
// transports run the replica controller, and the Space body controller.
func startTestbed(ctx context.Context, t *testing.T) *testbed.Testbed {
	// Register the factories.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	replicaFactory := func(b bus.Bus) controller.Factory { return space_world_replica_controller.NewFactory(b) }
	tb.StaticResolver.AddFactory(provider_local.NewFactory(tb.Bus, transport.WithChildFactories(replicaFactory)))
	tb.StaticResolver.AddFactory(space_sobject.NewFactory(tb.Bus))

	// Load the provider and the Space body controller.
	loadConfig(ctx, t, tb.Bus, &provider_local.Config{
		ProviderId: "local",
		PeerId:     tb.Volume.GetPeerID().String(),
		StorageId:  tb.StorageID,
	})
	loadConfig(ctx, t, tb.Bus, &space_sobject.Config{})
	return tb
}

// setupAccount creates an account and Session on the testbed's provider and
// starts its Session transport.
func setupAccount(ctx context.Context, t *testing.T, tb *testbed.Testbed) (*provider_local.ProviderAccount, *provider_local.Session) {
	// Look up the provider.
	t.Helper()
	prov, provRef, err := provider.ExLookupProvider(ctx, tb.Bus, "local", false, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(provRef.Release)

	// Create and access the account.
	sessRef, err := prov.(*provider_local.Provider).CreateLocalAccountAndSession(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	accIface, releaseAcc, err := prov.(*provider_local.Provider).AccessProviderAccount(ctx, sessRef.GetProviderResourceRef().GetProviderAccountId(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseAcc)
	acc := accIface.(*provider_local.ProviderAccount)

	// Mount the Session and start its transport.
	sess, releaseSess, err := acc.MountSession(ctx, sessRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(releaseSess)
	session := sess.(*provider_local.Session)
	if err := acc.EnsureConfiguredSessionTransport(ctx, session.GetPrivKey()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(acc.StopSessionTransport)
	t.Cleanup(acc.StopP2PSync)
	return acc, session
}

// The keys of the Flowgraph World's objects.
const (
	// flowgraphKey is the Flowgraph placing the replica pair.
	flowgraphKey = "flowgraph/replica"
	// deviceAKey is the Device object of account A's Session.
	deviceAKey = "device/a"
	// deviceBKey is the Device object of account B's Session.
	deviceBKey = "device/b"
)

// startFlowgraph seeds the testbed World with a Device object for each
// Session and an empty Flowgraph, and runs a reconciler for each Device that
// allows the replica node type. It returns the World's engine.
func startFlowgraph(ctx context.Context, t *testing.T, tb *testbed.Testbed, aPeer, bPeer string) world.Engine {
	// Serve the node types on the root bus.
	t.Helper()
	release, err := tb.Bus.AddController(ctx, flowgraph_nodetype.NewController(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)

	// Seed both Devices and the Flowgraph.
	devices := map[string]string{deviceAKey: aPeer, deviceBKey: bPeer}
	commit(ctx, t, tb.Engine, func(tx world.Tx) error {
		for key, peerID := range devices {
			obj, _, err := world.CreateWorldObject(ctx, tx, key, func(cursor *block.Cursor) error {
				cursor.SetBlock(&s4wave_device.Device{PeerId: peerID, Label: key}, true)
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
		_, _, err := tx.ApplyWorldOp(ctx, &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: flowgraphKey, Name: "replica"}, "")
		return err
	})

	// Allow the replica node type on each Device and run its reconciler.
	stateRoot := t.TempDir()
	if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{NodeTypeId: []string{space_world_replica.NodeTypeID}}); err != nil {
		t.Fatal(err)
	}
	policy, err := device_policy.NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	for key, peerID := range devices {
		r := device_flowgraph.NewReconciler(tb.Logger, tb.Bus, tb.Engine, tb.EngineID, policy, device_flowgraph.NewForgeWorkerWatch(), key, peerID, func() func() { return func() {} })
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan struct{})
		go func() {
			defer close(done)
			_ = r.Run(runCtx)
		}()
		t.Cleanup(func() { stop(); <-done })
	}
	return tb.Engine
}

// placePair sets replica nodes "a" and "b" on Devices A and B in the Flowgraph,
// joined by one connection, with desired as the desired owner node.
func placePair(ctx context.Context, t *testing.T, engine world.Engine, id, desired string) {
	// Build both nodes with the same parameters.
	t.Helper()
	node := func() *s4wave_flowgraph.FlowgraphNode {
		return &s4wave_flowgraph.FlowgraphNode{
			TypeId: space_world_replica.NodeTypeID,
			Parameters: map[string]string{
				space_world_replica.ParamSharedObject: id,
				space_world_replica.ParamDesiredOwner: desired,
			},
			Ports: space_world_replica.NodeType{}.GetPorts(),
		}
	}
	placement := func(key string) *s4wave_flowgraph.FlowgraphPlacement {
		return &s4wave_flowgraph.FlowgraphPlacement{Kind: s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: key}
	}

	// Write the pair, its connection and its placements.
	commit(ctx, t, engine, func(tx world.Tx) error {
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, flowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{
			SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"a": node(), "b": node()},
			SetConnections: map[string]*s4wave_flowgraph.FlowgraphConnection{"c1": {
				OutputNode: "a", OutputPort: space_world_replica.PortOut,
				InputNode: "b", InputPort: space_world_replica.PortIn,
			}},
			SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{"a": {Copies: []*s4wave_flowgraph.FlowgraphPlacement{placement(deviceAKey)}}, "b": {Copies: []*s4wave_flowgraph.FlowgraphPlacement{placement(deviceBKey)}}},
		})
		return err
	})
}

// commit runs fn in a write transaction of engine and commits it.
func commit(ctx context.Context, t *testing.T, engine world.Engine, fn func(tx world.Tx) error) {
	// Run fn in a write transaction and commit it.
	t.Helper()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := fn(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// waitOwner waits until acc lists id and its config names owner as an owner
// and no longer names departed.
func waitOwner(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, id, owner, departed string) {
	// Mount the listed SharedObject.
	t.Helper()
	so, release := mountListed(ctx, t, acc, id)
	defer release()
	states, releaseStates, err := so.GetSOHost().GetSOStateCtr(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseStates()

	// Wait for the config that moved the ownership.
	if _, err := states.WaitValueWithValidator(ctx, func(state *sobject.SOState) (bool, error) {
		conf := state.GetConfig()
		gone := !slices.ContainsFunc(conf.GetParticipants(), func(p *sobject.SOParticipantConfig) bool {
			return p.GetPeerId() == departed
		})
		return gone && slices.Contains(conf.SigningOwners(), owner), nil
	}, nil); err != nil {
		t.Fatalf("ownership did not move to %s: %v", owner, err)
	}
}

// mountListed waits until acc lists id and mounts it until release.
func mountListed(ctx context.Context, t *testing.T, acc *provider_local.ProviderAccount, id string) (*provider_local.SharedObject, func()) {
	// Wait for the entry.
	t.Helper()
	list, err := acc.GetSOListCtr().WaitValueWithValidator(ctx, func(list *sobject.SharedObjectList) (bool, error) {
		return listedRef(list, id) != nil, nil
	}, nil)
	if err != nil {
		t.Fatalf("account did not list the SharedObject: %v", err)
	}

	// Mount it.
	mounted, release, err := acc.MountSharedObject(ctx, listedRef(list, id), nil)
	if err != nil {
		t.Fatal(err)
	}
	return mounted.(*provider_local.SharedObject), release
}

// listedRef returns the reference of id in list, or nil.
func listedRef(list *sobject.SharedObjectList, id string) *sobject.SharedObjectRef {
	for _, entry := range list.GetSharedObjects() {
		if entry.GetRef().GetProviderResourceRef().GetId() == id {
			return entry.GetRef()
		}
	}
	return nil
}

// writeObject creates the object key in the World of the Space ref.
func writeObject(ctx context.Context, t *testing.T, b bus.Bus, ref *sobject.SharedObjectRef, key string) {
	// Mount the Space body only while writing.
	t.Helper()
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, b, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyRef.Release()

	// Create the object.
	ws := world.NewEngineWorldState(body.GetSharedObjectBody().GetWorldEngine(), true)
	obj, err := ws.CreateObject(ctx, key, nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
}

// checkObject checks that the World of id, as acc lists it, holds key.
func checkObject(ctx context.Context, t *testing.T, b bus.Bus, acc *provider_local.ProviderAccount, id, key string) {
	// Mount the Space body acc lists.
	t.Helper()
	ref := listedRef(acc.GetSOListCtr().GetValue(), id)
	body, bodyRef, err := space.ExMountSpaceSoBody(ctx, b, ref, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bodyRef.Release()

	// Look up the object.
	ws := world.NewEngineWorldState(body.GetSharedObjectBody().GetWorldEngine(), false)
	obj, found, err := ws.GetObject(ctx, key)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatalf("the new owner's World lacks %s", key)
	}
}

// mustSpaceMeta returns the metadata of a new Space.
func mustSpaceMeta(t *testing.T) *sobject.SharedObjectMeta {
	t.Helper()
	meta, err := space.NewSharedObjectMeta("Replica")
	if err != nil {
		t.Fatal(err)
	}
	return meta
}

// loadConfig loads conf on b for the rest of the test.
func loadConfig(ctx context.Context, t *testing.T, b bus.Bus, conf config.Config) {
	t.Helper()
	_, ref, err := b.AddDirective(ctx, resolver.NewLoadControllerWithConfig(conf), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
}
