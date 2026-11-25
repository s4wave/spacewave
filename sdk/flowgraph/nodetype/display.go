package flowgraph_nodetype

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	display_service "github.com/s4wave/spacewave/core/display"
	"github.com/s4wave/spacewave/core/frametrack"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// displayCodecParameter names the test pattern codec on a display node.
const displayCodecParameter = "codec"

// displaySourceParameter selects the capture source on a display node.
const displaySourceParameter = "source"

// display serves a display track on the Device that runs the node.
type display struct{}

// GetDisplayName returns the name shown for the node type.
func (display) GetDisplayName() string {
	return "Display"
}

// GetPorts returns no ports because viewers reach the display through the Device.
func (display) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns the configuration ID of the display controller.
func (display) GetConfigIDs() []string {
	return []string{display_service.ConfigID}
}

// Compile starts the display controller on the Device's Session.
func (display) Compile(_ context.Context, node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	// Use the AVC clip by default. JPEG renders the viewer's pointer.
	codec := frametrack.Codec_CODEC_H264_AVC
	switch node.Node.GetParameters()[displayCodecParameter] {
	case "", "h264":
	case "jpeg":
		codec = frametrack.Codec_CODEC_JPEG
	default:
		return nil, errors.New("display codec must be h264 or jpeg")
	}

	// Leave the existing patterns as the default and make GPU capture explicit.
	var source display_service.Source
	switch node.Node.GetParameters()[displaySourceParameter] {
	case "":
	case "gstreamer-test":
		source = display_service.Source_SOURCE_GSTREAMER_TEST
	default:
		return nil, errors.New("unknown display source")
	}

	// Validate the source and rendition before placing the display controller.
	conf := &display_service.Config{Codec: codec, CapabilityId: node.CapabilityID(), Source: source}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return map[string]config.Config{
		"display": conf,
	}, nil
}

// GetCapability publishes the protocol that serves pictures and accepts input.
func (display) GetCapability(_ context.Context, _ world.WorldState, node *s4wave_flowgraph.PlacedFlowgraphNode) (*s4wave_device.DeviceCapability, error) {
	// Address this placement so a retired node cannot answer the open.
	id := node.CapabilityID()
	return &s4wave_device.DeviceCapability{
		Kind:  s4wave_device.DeviceCapabilityKindDisplay,
		Label: "Display",
		Link:  &s4wave_device.DeviceCapabilityLink{ProtocolId: string(display_service.ControlProtocolID(id))},
		Policy: &s4wave_device.DeviceCapabilityPolicy{
			LocalPolicyRef: id,
			LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
			GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
		},
	}, nil
}

// _ is a type assertion.
var (
	_ s4wave_flowgraph.FlowgraphNodeType       = display{}
	_ s4wave_flowgraph.FlowgraphNodeCapability = display{}
)
