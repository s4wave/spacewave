package flowgraph_nodetype

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/checkoutroot"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

const (
	// checkoutRootNameParameter names the parameter holding the checkout root
	// name that selectors match.
	checkoutRootNameParameter = "name"
	// checkoutRootPathParameter names the parameter holding the absolute path of
	// the checkout root on its Device.
	checkoutRootPathParameter = "path"
	// checkoutRootAccessParameter names the parameter holding the access mode.
	checkoutRootAccessParameter = "access"
	// checkoutRootReadOnly is the access parameter value of a read-only root.
	checkoutRootReadOnly = "read-only"
	// checkoutRootReadWrite is the access parameter value of a root that accepts writes.
	checkoutRootReadWrite = "read-write"
)

// checkoutRoot serves a Device directory selected through its capabilities.
type checkoutRoot struct{}

// GetDisplayName returns the name shown for the node type.
func (checkoutRoot) GetDisplayName() string {
	return "Checkout Root"
}

// GetPorts returns no ports, since a checkout root connects to nothing.
func (checkoutRoot) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns the configuration ID of the checkout-root controller.
func (checkoutRoot) GetConfigIDs() []string {
	return []string{checkoutroot.ConfigID}
}

// Compile starts the folder's UnixFS server on the Device's Session bus.
func (checkoutRoot) Compile(_ context.Context, node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	// Load the root and access mode from the node declaration.
	payload, err := checkoutRootPayload(node.Node)
	if err != nil {
		return nil, err
	}

	// Configure the controller that owns the capability's protocol.
	return map[string]config.Config{"checkout-root": newCheckoutRootConfig(node.CapabilityID(), payload)}, nil
}

// GetCapability returns the filesystem capability that advertises the root.
func (checkoutRoot) GetCapability(
	_ context.Context,
	_ world.WorldState,
	node *s4wave_flowgraph.PlacedFlowgraphNode,
) (*s4wave_device.DeviceCapability, error) {
	// Read the root from the node's parameters.
	payload, err := checkoutRootPayload(node.Node)
	if err != nil {
		return nil, err
	}

	// Identify the root by its node, for the local policy and for selectors.
	id := node.CapabilityID()
	payload.SelectionRef = id
	return &s4wave_device.DeviceCapability{
		Kind:  s4wave_device.DeviceCapabilityKindFilesystem,
		Label: payload.GetName() + " checkout",
		Link:  &s4wave_device.DeviceCapabilityLink{ProtocolId: string(newCheckoutRootConfig(id, payload).ProtocolID())},
		Policy: &s4wave_device.DeviceCapabilityPolicy{
			LocalPolicyRef: id,
			LocalState:     s4wave_device.DeviceCapabilityLocalState_DEVICE_CAPABILITY_LOCAL_STATE_ENABLED,
			GrantState:     s4wave_device.DeviceCapabilityGrantState_DEVICE_CAPABILITY_GRANT_STATE_ALLOWED,
		},
		CheckoutRoot: payload,
	}, nil
}

// checkoutRootPayload returns the checkout root the node's parameters declare,
// or an error naming the parameter that is missing or malformed. The root is
// readable, and writable when its access is read-write.
func checkoutRootPayload(node *s4wave_flowgraph.FlowgraphNode) (*s4wave_device.DeviceCheckoutRootCapability, error) {
	// Read the parameters the declaration comes from.
	params := node.GetParameters()

	// Require a name for selectors to match.
	name := strings.TrimSpace(params[checkoutRootNameParameter])
	if name == "" {
		return nil, errors.Errorf("parameter %q is required", checkoutRootNameParameter)
	}

	// Require an absolute path, since a relative one names a different directory
	// for each process.
	path := strings.TrimSpace(params[checkoutRootPathParameter])
	if !filepath.IsAbs(path) {
		return nil, errors.Errorf("parameter %q must be an absolute path", checkoutRootPathParameter)
	}

	// Require one of the access modes.
	payload := &s4wave_device.DeviceCheckoutRootCapability{
		Name:          name,
		DisplayPath:   filepath.Clean(path),
		ReadAvailable: true,
	}
	switch params[checkoutRootAccessParameter] {
	case checkoutRootReadOnly:
		payload.Access = s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_ONLY
	case checkoutRootReadWrite:
		payload.Access = s4wave_device.DeviceCheckoutRootAccess_DEVICE_CHECKOUT_ROOT_ACCESS_READ_WRITE
		payload.WriteAvailable = true
	default:
		return nil, errors.Errorf("parameter %q must be %q or %q", checkoutRootAccessParameter, checkoutRootReadOnly, checkoutRootReadWrite)
	}
	return payload, nil
}

// newCheckoutRootConfig identifies a placed root and the access it serves.
func newCheckoutRootConfig(id string, payload *s4wave_device.DeviceCheckoutRootCapability) *checkoutroot.Config {
	return &checkoutroot.Config{
		CapabilityId: id,
		Path:         payload.GetDisplayPath(),
		Writable:     s4wave_device.DeviceCheckoutRootCanWrite(payload),
	}
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = checkoutRoot{}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeCapability = checkoutRoot{}
