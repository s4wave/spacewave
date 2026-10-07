package flowgraph_nodetype

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	terminal_remoteshell "github.com/s4wave/spacewave/core/terminal/remoteshell"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// remoteShellEntry names the one entry a Remote Shell compiles to.
const remoteShellEntry = "shell"

// remoteShell serves the remote shell of its Device to the peers of the
// Device's account. A Device whose Flowgraph does not place the node serves no
// shell.
type remoteShell struct{}

// GetDisplayName returns the name shown for the node type.
func (remoteShell) GetDisplayName() string {
	return "Remote Shell"
}

// GetPorts returns no ports, since the shell is reached through the Device.
func (remoteShell) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns the remote shell controller config ID.
func (remoteShell) GetConfigIDs() []string {
	return []string{terminal_remoteshell.ConfigID}
}

// Compile runs the remote shell controller on the Device's Session.
func (remoteShell) Compile(*s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	return map[string]config.Config{remoteShellEntry: &terminal_remoteshell.Config{}}, nil
}

// GetCapability returns the remote shell capability, which a Device keeps once.
func (remoteShell) GetCapability(
	_ context.Context,
	_ world.WorldState,
	node *s4wave_flowgraph.PlacedFlowgraphNode,
) (*s4wave_device.DeviceCapability, error) {
	return &s4wave_device.DeviceCapability{
		Kind:  s4wave_device.DeviceCapabilityKindRemoteShell,
		Label: "Remote Shell",
		Policy: &s4wave_device.DeviceCapabilityPolicy{
			LocalPolicyRef: node.CapabilityID(),
			LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
			GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
		},
	}, nil
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = remoteShell{}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeCapability = remoteShell{}
