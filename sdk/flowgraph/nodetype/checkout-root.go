package flowgraph_nodetype

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
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
	// checkoutRootReadWrite is the access parameter value of a root that writes
	// after approval.
	checkoutRootReadWrite = "read-write"
)

// checkoutRoot advertises a directory of its Device as a checkout root. It runs
// nothing, so its node reads AVAILABLE; consumers select the root through the
// Device's capabilities.
type checkoutRoot struct{}

// GetDisplayName returns the name shown for the node type.
func (checkoutRoot) GetDisplayName() string {
	return "Checkout Root"
}

// GetPorts returns no ports, since a checkout root connects to nothing.
func (checkoutRoot) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns no config IDs, since a checkout root compiles to no
// entries.
func (checkoutRoot) GetConfigIDs() []string {
	return nil
}

// Compile validates the node's parameters and compiles to no entries.
func (checkoutRoot) Compile(node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	_, err := checkoutRootPayload(node.Node)
	return nil, err
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
// readable, and writable after approval when its access is read-write.
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

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = checkoutRoot{}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeCapability = checkoutRoot{}
