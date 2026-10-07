//go:build !js

package spacewave_cli

import (
	"context"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_nodetype "github.com/s4wave/spacewave/sdk/flowgraph/nodetype"
	"github.com/sirupsen/logrus"
)

// TestRunFlowgraphReconcilerStartsWhenSetupCompletes starts the reconciler
// before the Device setup is session-ready and requires it to run the placed
// node once setup completes, without a restart.
func TestRunFlowgraphReconcilerStartsWhenSetupCompletes(t *testing.T) {
	// Bound the test, since a missing start would wait forever.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Seed this daemon's Device and a Flowgraph placing a TCP Port on it.
	const deviceKey = "device/self"
	const flowgraphKey = "flowgraph/main"
	tb := world_testbed.MustDefault(t, ctx)
	engine := tb.Engine
	p, _, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	peerID := p.GetPeerID().String()
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, _, err := world.CreateWorldObject(ctx, tx, deviceKey, func(cursor *block.Cursor) error {
		cursor.SetBlock(&s4wave_device.Device{PeerId: peerID, Label: deviceKey}, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, tx, deviceKey, s4wave_device.DeviceTypeID); err != nil {
		t.Fatal(err)
	}
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
		SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacement{"tcp": {
			Kind:      s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE,
			ObjectKey: deviceKey,
		}},
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
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
	mount := func(context.Context, *deviceSetupRecord) (world.Engine, func(), error) {
		return engine, func() {}, nil
	}
	runErr := make(chan error, 1)
	go func() { runErr <- runFlowgraphReconciler(runCtx, le, statePath, b, mount, store, func() func() { return func() {} }) }()

	// Complete the setup and require the node to show on the Device.
	if err := writeDeviceSetupRecord(statePath, &deviceSetupRecord{
		SetupState:      deviceSetupStateSessionReady,
		PeerID:          peerID,
		ResourceID:      "c3BhY2UtMQ==",
		SessionIndex:    1,
		DeviceObjectKey: deviceKey,
	}); err != nil {
		t.Fatal(err)
	}
	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for {
		rtx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		obj, _, err := rtx.GetObject(ctx, deviceKey)
		if err != nil {
			t.Fatal(err)
		}
		var device *s4wave_device.Device
		_, _, err = world.AccessObjectState(ctx, obj, false, func(cursor *block.Cursor) error {
			var err error
			device, err = s4wave_device.UnmarshalDevice(ctx, cursor)
			return err
		})
		world.ReleaseObjectState(obj)
		rtx.Discard()
		if err != nil {
			t.Fatal(err)
		}
		if len(device.GetCapabilities()) == 1 {
			break
		}
		if seqno, err = engine.WaitSeqno(ctx, seqno+1); err != nil {
			t.Fatalf("device never showed the placed node: %v", err)
		}
	}

	// Stop the reconciler.
	stop()
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}
}
