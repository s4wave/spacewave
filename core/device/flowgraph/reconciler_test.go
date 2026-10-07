//go:build !js

package device_flowgraph

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus/inmem"
	directive_controller "github.com/aperturerobotics/controllerbus/directive/controller"
	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	flowgraph_nodetype "github.com/s4wave/spacewave/sdk/flowgraph/nodetype"
	"github.com/sirupsen/logrus"
)

const (
	testFlowgraphKey = "flowgraph/main"
	testSelfKey      = "device/self"
	testOtherKey     = "device/other"
)

// TestReconciler runs a Reconciler for one Device against a World holding a
// Flowgraph that places a TCP Port and a Local Port on it, and checks the
// node state in the Device object through each policy, graph, and Session
// change.
func TestReconciler(t *testing.T) {
	// Bound the test, since a missing change would wait forever.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Seed this daemon's Device, another Device, and the Flowgraph.
	tb := world_testbed.MustDefault(t, ctx)
	engine := tb.Engine
	selfKey, selfPeer := newSessionPeer(t)
	otherPeer := newPeerID(t)
	commit(t, ctx, engine, func(tx world.Tx) error {
		for key, peerID := range map[string]string{testSelfKey: selfPeer, testOtherKey: otherPeer} {
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
		_, _, err := tx.ApplyWorldOp(ctx, &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: testFlowgraphKey, Name: "main"}, "")
		return err
	})

	// Allow only the TCP Port.
	stateRoot := t.TempDir()
	if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{NodeTypeId: []string{s4wave_flowgraph.TCPPortNodeTypeID}}); err != nil {
		t.Fatal(err)
	}
	policy, err := device_policy.NewPolicyStore(stateRoot)
	if err != nil {
		t.Fatal(err)
	}

	// Serve the node types on the root bus.
	b := inmem.NewBus(directive_controller.NewController(ctx, le))
	release, err := b.AddController(ctx, flowgraph_nodetype.NewController(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	// Run the Device's Session transport, whose bus hosts the node controllers.
	st, err := transport.NewSessionTransport(le, b, selfKey, "", "")
	if err != nil {
		t.Fatal(err)
	}
	sessionCtx, stopSession := context.WithCancel(ctx)
	sessionDone := make(chan struct{})
	go func() {
		defer close(sessionDone)
		_ = st.Execute(sessionCtx)
	}()
	if err := st.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	// Run the Reconciler until the test stops it, counting its holds on the
	// daemon.
	var holds atomic.Int32
	hold := func() func() {
		holds.Add(1)
		return func() { holds.Add(-1) }
	}
	runCtx, stop := context.WithCancel(ctx)
	runErr := make(chan error, 1)
	go func() {
		runErr <- NewReconciler(le, b, engine, policy, testSelfKey, selfPeer, hold).Run(runCtx)
	}()

	// Place a TCP Port and a Local Port on this Device. Each connects to a node
	// on the other Device, so this Device runs the forwarding end of one
	// connection and the listening end of the other.
	commit(t, ctx, engine, func(tx world.Tx) error {
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{
			SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{
				"tcp-self":    portNode(s4wave_flowgraph.TCPPortNodeTypeID, "127.0.0.1:8080", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT),
				"local-other": portNode(s4wave_flowgraph.LocalPortNodeTypeID, "127.0.0.1:0", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT),
				"local-self":  portNode(s4wave_flowgraph.LocalPortNodeTypeID, "127.0.0.1:0", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT),
				"tcp-other":   portNode(s4wave_flowgraph.TCPPortNodeTypeID, "127.0.0.1:8081", s4wave_flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT),
			},
			SetConnections: map[string]*s4wave_flowgraph.FlowgraphConnection{
				"out": {OutputNode: "tcp-self", OutputPort: "stream", InputNode: "local-other", InputPort: "stream"},
				"in":  {OutputNode: "tcp-other", OutputPort: "stream", InputNode: "local-self", InputPort: "stream"},
			},
			SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacement{
				"tcp-self":    devicePlacement(testSelfKey),
				"local-self":  devicePlacement(testSelfKey),
				"local-other": devicePlacement(testOtherKey),
				"tcp-other":   devicePlacement(testOtherKey),
			},
		})
		return err
	})

	// Require the disallowed Local Port to show as rejected while the TCP Port
	// shows as active.
	capabilities := waitCapabilities(t, ctx, engine, func(capabilities map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(capabilities, "tcp-self") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE &&
			capabilityState(capabilities, "local-self") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED
	})
	if detail := capabilities[capabilityID("local-self")].GetDetail(); !strings.Contains(detail, "not allowed") {
		t.Fatalf("rejected node detail %q does not state the policy", detail)
	}
	if len(capabilities) != 2 {
		t.Fatalf("device holds %d node capabilities, want only the nodes placed on it", len(capabilities))
	}
	if n := holds.Load(); n != 1 {
		t.Fatalf("reconciler holds the daemon %d times while a node runs, want 1", n)
	}

	// Allow the Local Port and require both ends to run.
	if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{
		NodeTypeId: []string{s4wave_flowgraph.TCPPortNodeTypeID, s4wave_flowgraph.LocalPortNodeTypeID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := policy.Reload(); err != nil {
		t.Fatal(err)
	}
	waitCapabilities(t, ctx, engine, func(capabilities map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(capabilities, "tcp-self") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE &&
			capabilityState(capabilities, "local-self") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE
	})

	// Remove the connection from the TCP Port and require its node available
	// while the other connection keeps running.
	commit(t, ctx, engine, func(tx world.Tx) error {
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{
			RemoveConnectionIds: []string{"out"},
		})
		return err
	})
	waitCapabilities(t, ctx, engine, func(capabilities map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(capabilities, "tcp-self") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE &&
			capabilityState(capabilities, "local-self") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE
	})

	// Restart the Session transport and require the Local Port to run again on
	// the new Session bus.
	stopSession()
	<-sessionDone
	waitCapabilities(t, ctx, engine, func(capabilities map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(capabilities, "local-self") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED
	})
	go func() { _ = st.Execute(ctx) }()
	waitCapabilities(t, ctx, engine, func(capabilities map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(capabilities, "local-self") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE
	})

	// Allow the Checkout Root and place three on this Device: one that
	// advertises the skiffos root, one with a relative path, and one that
	// repeats the skiffos name.
	if err := device_policy.WriteFile(stateRoot, &device_policy.DevicePolicy{
		NodeTypeId: []string{s4wave_flowgraph.LocalPortNodeTypeID, s4wave_flowgraph.CheckoutRootNodeTypeID},
	}); err != nil {
		t.Fatal(err)
	}
	if err := policy.Reload(); err != nil {
		t.Fatal(err)
	}

	// Place the roots.
	commit(t, ctx, engine, func(tx world.Tx) error {
		_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, tx, testFlowgraphKey, &s4wave_flowgraph.UpdateFlowgraphRequest{
			SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{
				"root-a":        checkoutRootNode("skiffos", "/srv/skiffos", "read-write"),
				"root-b":        checkoutRootNode("skiffos", "/srv/other", "read-only"),
				"root-relative": checkoutRootNode("relative", "srv/relative", "read-only"),
			},
			SetPlacements: map[string]*s4wave_flowgraph.FlowgraphPlacement{
				"root-a":        devicePlacement(testSelfKey),
				"root-b":        devicePlacement(testSelfKey),
				"root-relative": devicePlacement(testSelfKey),
			},
		})
		return err
	})

	// Require the first root to show as available, and the others as rejected.
	capabilities = waitCapabilities(t, ctx, engine, func(capabilities map[string]*s4wave_device.DeviceCapability) bool {
		return capabilityState(capabilities, "root-a") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE &&
			capabilityState(capabilities, "root-b") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED &&
			capabilityState(capabilities, "root-relative") == s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED
	})
	rootCap := capabilities[capabilityID("root-a")]
	if rootCap.GetKind() != s4wave_device.DeviceCapabilityKindFilesystem {
		t.Fatalf("checkout root capability kind = %q, want %q", rootCap.GetKind(), s4wave_device.DeviceCapabilityKindFilesystem)
	}
	if detail := capabilities[capabilityID("root-b")].GetDetail(); !strings.Contains(detail, testFlowgraphKey+"/root-a") {
		t.Fatalf("duplicate root detail %q does not name the node that keeps the root", detail)
	}

	// Require the Device to select only the root that is available.
	device := &s4wave_device.Device{}
	for _, capability := range capabilities {
		device.Capabilities = append(device.Capabilities, capability)
	}
	if root := device.FindSelectableCheckoutRoot("skiffos"); root.GetId() != capabilityID("root-a") {
		t.Fatalf("selectable skiffos root = %v, want the capability of root-a", root)
	}
	if root := device.FindSelectableCheckoutRoot("relative"); root != nil {
		t.Fatalf("selectable relative root = %v, want none", root)
	}

	// Stop the Reconciler and require it to release the daemon.
	stop()
	if err := <-runErr; err != nil {
		t.Fatal(err)
	}
	if n := holds.Load(); n != 0 {
		t.Fatalf("reconciler holds the daemon %d times after it stops, want 0", n)
	}
}

// newSessionPeer returns the private key and ID of a new peer.
func newSessionPeer(t *testing.T) (crypto.PrivKey, string) {
	t.Helper()
	p, key, _, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	return key, p.GetPeerID().String()
}

// newPeerID returns the ID of a new peer.
func newPeerID(t *testing.T) string {
	t.Helper()
	_, id := newSessionPeer(t)
	return id
}

// commit applies fn in one committed write transaction.
func commit(t *testing.T, ctx context.Context, engine world.Engine, fn func(tx world.Tx) error) {
	// Report failures at the caller.
	t.Helper()

	// Run fn in a write transaction and commit it.
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

// portNode returns a TCP Port or Local Port node with one stream port.
func portNode(typeID, address string, direction s4wave_flowgraph.FlowgraphPortDirection) *s4wave_flowgraph.FlowgraphNode {
	return &s4wave_flowgraph.FlowgraphNode{
		TypeId:     typeID,
		Parameters: map[string]string{"address": address},
		Ports:      []*s4wave_flowgraph.FlowgraphPort{{Name: "stream", Direction: direction, TypeId: "stream"}},
	}
}

// checkoutRootNode returns a Checkout Root node.
func checkoutRootNode(name, path, access string) *s4wave_flowgraph.FlowgraphNode {
	return &s4wave_flowgraph.FlowgraphNode{
		TypeId:     s4wave_flowgraph.CheckoutRootNodeTypeID,
		Parameters: map[string]string{"name": name, "path": path, "access": access},
	}
}

// devicePlacement returns a placement on the Device at key.
func devicePlacement(key string) *s4wave_flowgraph.FlowgraphPlacement {
	return &s4wave_flowgraph.FlowgraphPlacement{
		Kind:      s4wave_flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE,
		ObjectKey: key,
	}
}

// capabilityID returns the ID of the capability that shows a node of the test
// Flowgraph.
func capabilityID(nodeID string) string {
	return s4wave_device.DeviceCapabilityKindFlowgraphNode + "/" + testFlowgraphKey + "/" + nodeID
}

// capabilityState returns the state of the node's capability, or UNKNOWN while
// the Device holds none.
func capabilityState(capabilities map[string]*s4wave_device.DeviceCapability, nodeID string) s4wave_device.DeviceCapabilityState {
	return capabilities[capabilityID(nodeID)].GetState()
}

// waitCapabilities waits until the node capabilities of this daemon's Device
// satisfy ready, then returns them by ID. It reads the Device after every World
// change.
func waitCapabilities(
	t *testing.T,
	ctx context.Context,
	engine world.Engine,
	ready func(map[string]*s4wave_device.DeviceCapability) bool,
) map[string]*s4wave_device.DeviceCapability {
	t.Helper()
	seqno, err := engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for {
		tx, err := engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		obj, _, err := tx.GetObject(ctx, testSelfKey)
		if err != nil {
			t.Fatal(err)
		}
		device, err := readDevice(ctx, obj)
		world.ReleaseObjectState(obj)
		tx.Discard()
		if err != nil {
			t.Fatal(err)
		}

		capabilities := make(map[string]*s4wave_device.DeviceCapability)
		for _, capability := range device.GetCapabilities() {
			if s4wave_flowgraph.IsNodeCapabilityID(capability.GetId()) {
				capabilities[capability.GetId()] = capability
			}
		}
		if ready(capabilities) {
			return capabilities
		}
		if seqno, err = engine.WaitSeqno(ctx, seqno+1); err != nil {
			t.Fatalf("device capabilities never satisfied the check, last read %v: %v", capabilities, err)
		}
	}
}
