//go:build !js

package spacewave_cli

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	device_flowgraph "github.com/s4wave/spacewave/core/device/flowgraph"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	core_provider "github.com/s4wave/spacewave/core/provider"
	core_session "github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_nodetype "github.com/s4wave/spacewave/sdk/flowgraph/nodetype"
	"github.com/sirupsen/logrus"
)

// TestRunFlowgraphReconcilerStartsWhenSetupCompletes starts the reconciler
// before the Device setup is session-ready, in a Space holding no object for
// this Device, as one the Device just joined. Once setup completes, without a
// restart, the reconciler must write the Device object and then run a node
// placed on it.
func TestRunFlowgraphReconcilerStartsWhenSetupCompletes(t *testing.T) {
	// Bound the test, since a missing start would wait forever.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Name this daemon's Device by its session peer, as enrollment does.
	tb := world_testbed.MustDefault(t, ctx)
	engine := tb.Engine
	p, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	peerID := p.GetPeerID().String()
	deviceKey, err := deviceObjectKey(peerID)
	if err != nil {
		t.Fatal(err)
	}

	// Allow the TCP Port and serve the node types.
	statePath := t.TempDir()
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{NodeTypeId: []string{s4wave_flowgraph.TCPPortNodeTypeID}}); err != nil {
		t.Fatal(err)
	}
	store, err := device_policy.NewPolicyStore(statePath)
	if err != nil {
		t.Fatal(err)
	}
	b := inmem.NewBus(directive_controller.NewController(ctx, le))
	releaseTypes, err := b.AddController(ctx, flowgraph_nodetype.NewController(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseTypes()

	// Start the reconciler while the Device is not set up.
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	mount := func(context.Context, flowgraphTarget) (*servedWorld, error) {
		return &servedWorld{engine: engine, engineID: tb.EngineID, peerID: peerID, release: func() {}}, nil
	}
	sessions := staticSessionList(testSessionEntry(1, "account", "session"))
	runErr := make(chan error, 1)
	go func() {
		runErr <- runFlowgraphReconciler(runCtx, le, statePath, b, sessions, mount, store, device_flowgraph.NewForgeWorkerWatch(), func() func() { return func() {} })
	}()

	// Complete the setup and require the Device object to appear.
	if err := (&deviceSetupRecord{
		SetupState:      deviceSetupStateSessionReady,
		PeerID:          peerID,
		ResourceID:      "c3BhY2UtMQ==",
		AccountID:       "account",
		SessionID:       "session",
		DeviceObjectKey: deviceKey,
	}).write(statePath); err != nil {
		t.Fatal(err)
	}
	waitDevice(t, ctx, engine, deviceKey, func(device *s4wave_device.Device) bool { return device != nil })

	// Place a TCP Port on the Device and require the node to show on it.
	placeTCPPort(t, ctx, engine, deviceKey)
	waitDevice(t, ctx, engine, deviceKey, func(device *s4wave_device.Device) bool { return len(device.GetCapabilities()) == 1 })

	// Stop the reconciler.
	stop()
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}
}

// TestRunFlowgraphReconcilerFollowsSetupSessionByIdentity starts the
// reconciler with a session-ready setup whose session the session list lacks,
// while another session holds a lower index, as after the setup's session
// left the list. The setup Space must wait without mounting anything, then
// mount through the setup's session once the list holds it, and remount at
// the session's new index when it moves.
func TestRunFlowgraphReconcilerFollowsSetupSessionByIdentity(t *testing.T) {
	// Bound the test, since a missing mount would wait forever.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Write a session-ready setup and an empty policy.
	statePath := t.TempDir()
	if err := (&deviceSetupRecord{
		SetupState:      deviceSetupStateSessionReady,
		PeerID:          "peer",
		ResourceID:      "c3BhY2UtMQ==",
		AccountID:       "device-account",
		SessionID:       "device-session",
		DeviceObjectKey: "devices/key",
	}).write(statePath); err != nil {
		t.Fatal(err)
	}
	if err := device_policy.WriteFile(statePath, &device_policy.DevicePolicy{}); err != nil {
		t.Fatal(err)
	}
	store, err := device_policy.NewPolicyStore(statePath)
	if err != nil {
		t.Fatal(err)
	}

	// Report each mount, which holds until its Reconciler stops.
	mounts := make(chan flowgraphTarget, 8)
	mount := func(ctx context.Context, target flowgraphTarget) (*servedWorld, error) {
		mounts <- target
		<-ctx.Done()
		return nil, ctx.Err()
	}

	// Feed the session lists the test sends.
	lists := make(chan []*core_session.SessionListEntry)
	follow := func(ctx context.Context, set func([]*core_session.SessionListEntry)) error {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case list := <-lists:
				set(list)
			}
		}
	}
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	runErr := make(chan error, 1)
	go func() {
		b := inmem.NewBus(directive_controller.NewController(runCtx, le))
		runErr <- runFlowgraphReconciler(runCtx, le, statePath, b, follow, mount, store, device_flowgraph.NewForgeWorkerWatch(), func() func() { return func() {} })
	}()

	// A list without the setup's session mounts nothing, not even the
	// session at another index, and does not retry.
	other := testSessionEntry(4, "other-account", "other-session")
	lists <- []*core_session.SessionListEntry{other}
	select {
	case target := <-mounts:
		t.Fatalf("mounted %+v while the setup's session is gone", target)
	case <-time.After(time.Duration(flowgraphRetryBackoff.GetExponential().GetInitialInterval()) * time.Millisecond):
	}

	// The session's return mounts the setup Space through it.
	id := sessionIdentity{accountID: "device-account", sessionID: "device-session"}
	want := flowgraphTarget{sessionIndex: 7, session: id, spaceID: "space-1"}
	lists <- []*core_session.SessionListEntry{other, testSessionEntry(7, "device-account", "device-session")}
	if got := waitFlowgraphMount(t, ctx, mounts); got != want {
		t.Fatalf("mounted %+v, want %+v", got, want)
	}

	// A move to another index remounts at the new index.
	want.sessionIndex = 8
	lists <- []*core_session.SessionListEntry{other, testSessionEntry(8, "device-account", "device-session")}
	if got := waitFlowgraphMount(t, ctx, mounts); got != want {
		t.Fatalf("mounted %+v after the move, want %+v", got, want)
	}

	// Stop the reconciler.
	stop()
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}
}

// waitFlowgraphMount returns the next target the reconciler mounts.
func waitFlowgraphMount(t *testing.T, ctx context.Context, mounts <-chan flowgraphTarget) flowgraphTarget {
	select {
	case target := <-mounts:
		return target
	case <-ctx.Done():
		t.Fatal("reconciler did not mount the setup Space")
		return flowgraphTarget{}
	}
}

// testSessionEntry returns a session list entry naming a session at idx.
func testSessionEntry(idx uint32, accountID, sessionID string) *core_session.SessionListEntry {
	return &core_session.SessionListEntry{
		SessionIndex: idx,
		SessionRef: &core_session.SessionRef{ProviderResourceRef: &core_provider.ProviderResourceRef{
			Id:                sessionID,
			ProviderAccountId: accountID,
			ProviderId:        "local",
		}},
	}
}

// staticSessionList returns a follower reporting one session list that never
// changes.
func staticSessionList(entries ...*core_session.SessionListEntry) followSessionListFunc {
	return func(ctx context.Context, set func([]*core_session.SessionListEntry)) error {
		set(entries)
		<-ctx.Done()
		return ctx.Err()
	}
}

// placeTCPPort creates a Flowgraph placing a TCP Port node on deviceKey.
func placeTCPPort(t *testing.T, ctx context.Context, engine world.Engine, deviceKey string) {
	// Create the Flowgraph and place the node in one transaction.
	const flowgraphKey = "flowgraph/main"
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if _, _, err := tx.ApplyWorldOp(ctx, &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: flowgraphKey, Name: "main"}, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, flowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{
		SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"tcp": {
			TypeId:     s4wave_flowgraph.TCPPortNodeTypeID,
			Parameters: map[string]string{"address": "127.0.0.1:8080"},
			Ports: []*s4wave_flowgraph.FlowgraphPort{{
				Name:      "stream",
				Direction: s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT,
				TypeId:    "stream",
			}},
		}},
		SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacements{"tcp": {Copies: []*s4wave_flowgraph.FlowgraphPlacement{{
			Kind:      s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE,
			ObjectKey: deviceKey,
		}}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// waitDevice waits for the Device object deviceKey to satisfy done, which
// receives nil while the object is missing.
func waitDevice(t *testing.T, ctx context.Context, engine world.Engine, deviceKey string, done func(*s4wave_device.Device) bool) {
	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for {
		// Read the Device object at the current revision.
		rtx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		obj, found, err := rtx.GetObject(ctx, deviceKey)
		if err != nil {
			t.Fatal(err)
		}
		var device *s4wave_device.Device
		if found {
			_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
				var err error
				device, err = s4wave_device.UnmarshalDevice(ctx, cursor)
				return err
			})
		}
		world.ReleaseObjectState(obj)
		rtx.Discard()
		if err != nil {
			t.Fatal(err)
		}

		// Stop once done, or wait for the next revision.
		if done(device) {
			return
		}
		if seqno, err = engine.WaitSeqno(ctx, seqno+1); err != nil {
			t.Fatalf("device %s never reached the expected state: %v", deviceKey, err)
		}
	}
}
