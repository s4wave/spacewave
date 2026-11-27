// Package os_system is the os/system Flowgraph node: placed on a Device
// running Spacewave OS and bound to an os/release object, it moves the Device
// to the release's image. The node serves the release to the Device's
// console, which stages it beside the running revision and boots it on
// trial. A trial that fails boots back to the accepted revision.
package os_system

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// NodeTypeID selects the os/system node type. The node reads the os/release
// object its Device runs as its object: many Devices may follow one release,
// so the binding holds nothing.
const NodeTypeID = "os/system"

// NodeType is the os/system node type. Its node runs a system controller on
// the Device the node is placed on.
type NodeType struct{}

// GetDisplayName returns the name shown for the node type.
func (NodeType) GetDisplayName() string {
	return "System"
}

// GetPorts returns no ports, since the node binds its release as its object.
func (NodeType) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns the system controller config ID.
func (NodeType) GetConfigIDs() []string {
	return []string{ConfigID}
}

// Compile returns the system controller config of the node.
func (NodeType) Compile(_ context.Context, node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	// A held release would let only one Device follow it.
	if !node.Node.GetReads() {
		return nil, errors.New("an os/system node reads its release; set reads on the node")
	}
	conf := &Config{EngineId: node.EngineID, ObjectKey: node.Node.GetObjectKey()}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return map[string]config.Config{"system": conf}, nil
}

// GetCapability shows the node as the Device's system. A Device keeps the
// first node of each kind, so it runs one system node however many
// Flowgraphs place one.
func (NodeType) GetCapability(_ context.Context, _ world.WorldState, node *s4wave_flowgraph.PlacedFlowgraphNode) (*s4wave_device.DeviceCapability, error) {
	return &s4wave_device.DeviceCapability{
		Kind:  s4wave_device.DeviceCapabilityKindSystem,
		Label: "System",
		Policy: &s4wave_device.DeviceCapabilityPolicy{
			LocalPolicyRef: node.CapabilityID(),
			LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
			GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
		},
	}, nil
}

// _ is a type assertion
var (
	_ s4wave_flowgraph.FlowgraphNodeType       = NodeType{}
	_ s4wave_flowgraph.FlowgraphNodeCapability = NodeType{}
)
