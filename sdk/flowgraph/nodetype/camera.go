package flowgraph_nodetype

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	camera_service "github.com/s4wave/spacewave/core/camera"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// cameraDeviceParameter identifies the local capture device on a camera node.
const cameraDeviceParameter = "device"

// camera serves a camera track on the Device that runs the node.
type camera struct{}

// GetDisplayName returns the name shown for the node type.
func (camera) GetDisplayName() string {
	return "Camera"
}

// GetPorts returns no ports because viewers reach the camera through the Device.
func (camera) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns the camera controller config ID.
func (camera) GetConfigIDs() []string {
	return []string{camera_service.ConfigID}
}

// Compile starts the camera controller on the Device's Session.
func (camera) Compile(_ context.Context, node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	return map[string]config.Config{
		"camera": &camera_service.Config{Device: node.Node.GetParameters()[cameraDeviceParameter]},
	}, nil
}

// GetCapability publishes the protocol viewers use to subscribe to the camera.
func (camera) GetCapability(_ context.Context, _ world.WorldState, node *s4wave_flowgraph.PlacedFlowgraphNode) (*s4wave_device.DeviceCapability, error) {
	return &s4wave_device.DeviceCapability{
		Kind:  s4wave_device.DeviceCapabilityKindCamera,
		Label: "Camera",
		Link:  &s4wave_device.DeviceCapabilityLink{ProtocolId: string(camera_service.ControlProtocolID)},
		Policy: &s4wave_device.DeviceCapabilityPolicy{
			LocalPolicyRef: node.CapabilityID(),
			LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
			GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
		},
	}, nil
}

// _ is a type assertion.
var (
	_ s4wave_flowgraph.FlowgraphNodeType       = camera{}
	_ s4wave_flowgraph.FlowgraphNodeCapability = camera{}
)
