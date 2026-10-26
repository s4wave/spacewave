//go:build !js

package device_flowgraph

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	resource_configtype_registry "github.com/s4wave/spacewave/core/resource/configtype/registry"
	resource_flowgraph_registry "github.com/s4wave/spacewave/core/resource/flowgraph/registry"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	stream_echo "github.com/s4wave/spacewave/net/stream/echo"
	flowgraph_plugin "github.com/s4wave/spacewave/plugin/flowgraph"
	s4wave_configtype_registry "github.com/s4wave/spacewave/sdk/configtype/registry"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_registry "github.com/s4wave/spacewave/sdk/flowgraph/registry"
	"github.com/sirupsen/logrus"
)

// TestPluginNodeFixture runs an isolated Device through plugin arrival and withdrawal.
// The compiler crosses a bidirectional Resource stream and never joins the Device bus.
func TestPluginNodeFixture(t *testing.T) {
	// Create an isolated Device World and an allowed plugin node placement.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	tb := world_testbed.MustDefault(t, ctx)

	// Declare a Stream Echo node placed on that Device.
	key, peerID := newSessionPeer(t)
	node := &s4wave_flowgraph.FlowgraphNode{TypeId: flowgraph_plugin.StreamEchoNodeTypeID, Parameters: map[string]string{"protocol_id": "fixture/echo"}, Ports: flowgraph_plugin.NewStreamEcho().GetPorts()}
	commit(t, ctx, tb.Engine, func(tx world.Tx) error {
		// Seed the Device and graph through their ordinary World interfaces.
		obj, _, err := world.CreateWorldObject(ctx, tx, testSelfKey, func(cursor *block.Cursor) error {
			cursor.SetBlock(&s4wave_device.Device{PeerId: peerID, Label: "Plugin fixture", Platform: &s4wave_device.DevicePlatform{Os: "darwin", Arch: "arm64"}}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		if err := world_types.SetObjectType(ctx, tx, testSelfKey, s4wave_device.DeviceTypeID); err != nil {
			return err
		}
		if _, _, err := tx.ApplyWorldOp(ctx, &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: testFlowgraphKey, Name: "Plugin fixture"}, ""); err != nil {
			return err
		}
		_, err = s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{
			SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{"echo": node}, SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacement{"echo": devicePlacement(testSelfKey)},
		})
		return err
	})

	// Allow the Device policy to run the plugin's node type.
	stateRoot := t.TempDir()
	if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{NodeTypeId: []string{flowgraph_plugin.StreamEchoNodeTypeID}}); err != nil {
		t.Fatal(err)
	}
	policy, err := device_policy.NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve plugin declarations and config constructors on the daemon bus, with no plugin controller there.
	b := tb.Bus
	tb.StaticResolver.AddFactory(stream_echo.NewFactory(b))

	// Serve the node type registry through the bridge controller.
	registry := resource_flowgraph_registry.NewRegistryResource()
	bridgeRelease, err := b.AddController(ctx, resource_flowgraph_registry.NewBridgeController(b, registry), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bridgeRelease()

	// Run compiled controllers on the actual Device Session bus.
	st, err := transport.NewSessionTransport(le, b, key, "", "", transport.WithChildFactories(func(b bus.Bus) controller.Factory { return stream_echo.NewFactory(b) }))
	if err != nil {
		t.Fatal(err)
	}

	// Run the Session until the test ends.
	sessionDone := make(chan error, 1)
	go func() { sessionDone <- st.Execute(ctx) }()
	defer func() {
		cancel()
		if err := <-sessionDone; err != nil && err != context.Canceled {
			t.Error(err)
		}
	}()

	// Wait for the Session bus before reconciling.
	if err := st.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	// Reconcile the Device against the standing node type lookup.
	result := make(chan error, 1)
	r := NewReconciler(le, b, tb.Engine, tb.EngineID, policy, NewForgeWorkerWatch(), testSelfKey, peerID, func() func() { return func() {} })
	go func() { result <- r.Run(ctx) }()
	defer func() {
		cancel()
		if err := <-result; err != nil {
			t.Error(err)
		}
	}()

	// A Device without this plugin reports unavailable and applies no configs.
	missing := waitCapabilities(t, ctx, tb.Engine, func(caps map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(caps, "echo") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED
	})
	if detail := missing[capabilityID("echo")].GetDetail(); !strings.Contains(detail, "Node type unavailable") {
		t.Fatalf("missing plugin detail = %q", detail)
	}

	// Register the real plugin compiler and editor over the Resource RPC boundary.
	editors := resource_configtype_registry.NewConfigTypeRegistryResource()

	// Serve both registries as the core root Resource.
	rootMux := srpc.NewMux()
	if err := flowgraph_registry.SRPCRegisterNodeTypeRegistryResourceService(rootMux, registry); err != nil {
		t.Fatal(err)
	}
	if err := s4wave_configtype_registry.SRPCRegisterConfigTypeRegistryResourceService(rootMux, editors); err != nil {
		t.Fatal(err)
	}

	// Connect a Resource client the way the plugin does.
	serverMux := srpc.NewMux()
	if err := resource_server.NewResourceServer(rootMux).Register(serverMux); err != nil {
		t.Fatal(err)
	}
	caller := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(serverMux)))
	resources, err := resource_client.NewClient(ctx, resource.NewSRPCResourceServiceClient(caller))
	if err != nil {
		t.Fatal(err)
	}
	defer resources.Release()

	// Access the root Resource's registry services.
	root := resources.AccessRootResource()
	defer root.Release()
	rootClient, err := root.GetClient()
	if err != nil {
		t.Fatal(err)
	}

	// Register the plugin's compiler and editor.
	editorPath := "/p/spacewave-flowgraph/a/StreamEchoEditor.js"
	release, err := flowgraph_plugin.Register(ctx, resources, rootClient, editorPath)
	if err != nil {
		t.Fatal(err)
	}

	// Resolve metadata and compile on the host through the retained plugin capability.
	nodeType, ref, err := s4wave_flowgraph.ExLookupFlowgraphNodeType(ctx, b, node.GetTypeId())
	if err != nil {
		t.Fatal(err)
	}
	defer ref.Release()

	// Require the declared metadata.
	if nodeType.GetDisplayName() != "Stream Echo" || !slices.Equal(nodeType.GetConfigIDs(), []string{stream_echo.ConfigID}) {
		t.Fatalf("unexpected node declaration: %q, %v", nodeType.GetDisplayName(), nodeType.GetConfigIDs())
	}

	// Compile through the proxy and decode with the host's constructors.
	entries, err := nodeType.Compile(ctx, &s4wave_flowgraph.PlacedFlowgraphNode{FlowgraphKey: testFlowgraphKey, NodeID: "echo", Node: node, DevicePeerID: peerID})
	if err != nil {
		t.Fatal(err)
	}
	conf, ok := entries["protocol"].(*stream_echo.Config)
	if !ok || conf.GetPeerId() != peerID || conf.GetProtocolId() != "fixture/echo" {
		t.Fatalf("compiled echo config = %v", entries)
	}

	// Require the editor registration and the active Device capability.
	if editor := editors.LookupRegistration(stream_echo.ConfigID); editor.GetScriptPath() != editorPath || editor.GetPluginId() != flowgraph_plugin.PluginID {
		t.Fatalf("editor registration = %v", editor)
	}
	waitCapabilities(t, ctx, tb.Engine, func(caps map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(caps, "echo") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE
	})

	// Release the plugin and require the standing lookup to return to unavailable.
	if err := release(); err != nil {
		t.Fatal(err)
	}
	waitCapabilities(t, ctx, tb.Engine, func(caps map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(caps, "echo") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED
	})
	if editor := editors.LookupRegistration(stream_echo.ConfigID); editor != nil {
		t.Fatalf("released plugin retained editor %v", editor)
	}
	if _, err := nodeType.Compile(ctx, &s4wave_flowgraph.PlacedFlowgraphNode{Node: node, DevicePeerID: peerID}); err == nil {
		t.Fatal("released compiler still accepts placements")
	}
}
