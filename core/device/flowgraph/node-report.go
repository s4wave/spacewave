package device_flowgraph

import (
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// nodeReport is the outcome of compiling one placed node.
type nodeReport struct {
	// node is the placed node.
	node *s4wave_flowgraph.PlacedFlowgraphNode
	// nodeType is the node's resolved type, or nil while the node is not
	// allowed or no controller supplies the type.
	nodeType s4wave_flowgraph.FlowgraphNodeType
	// shown is the capability the node type shows for an accepted node, or nil
	// to show the default flowgraph-node capability.
	shown *s4wave_device.DeviceCapability
	// keys contains the ConfigSet key of each entry the node compiled to.
	keys []string
	// err is why the daemon rejects the node.
	err error
}

// capability returns the Device capability that shows the node's state, folded
// from the state of its entries.
func (n *nodeReport) capability(applier *configApplier) *s4wave_device.DeviceCapability {
	// Show the capability the node type supplied, or the default one.
	capability := n.shown.CloneVT()
	if capability == nil {
		// Label the default capability with the node type's name, or its ID
		// until the type resolves.
		label := n.node.Node.GetTypeId()
		if n.nodeType != nil {
			label = n.nodeType.GetDisplayName()
		}
		capability = &s4wave_device.DeviceCapability{
			Kind:  s4wave_device.DeviceCapabilityKindFlowgraphNode,
			Label: label + " " + n.node.NodeID,
			Link: &s4wave_device.DeviceCapabilityLink{
				ObjectKey: n.node.FlowgraphKey,
				TypeId:    s4wave_flowgraph.FlowgraphTypeID,
			},
		}
	}
	capability.Id = n.node.CapabilityID()
	capability.State, capability.Detail = n.state(applier)
	return capability
}

// state folds the node's entries into one state. Any failed entry rejects the
// node; the node is active once every entry's controller runs.
func (n *nodeReport) state(applier *configApplier) (s4wave_device.DeviceCapabilityState, string) {
	// Report a node the daemon rejected, or one still waiting for its type.
	if n.err != nil {
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED, n.err.Error()
	}
	if n.nodeType == nil {
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED, "waiting for the node type"
	}
	if len(n.keys) == 0 {
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE, ""
	}
	starting := false
	for _, key := range n.keys {
		state := applier.State(key)
		switch {
		case state != nil && state.GetError() != nil:
			return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_REJECTED, state.GetError().Error()
		case state == nil || state.GetController() == nil:
			starting = true
		}
	}
	if starting {
		return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_DECLARED, "starting"
	}
	return s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE, ""
}
