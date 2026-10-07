package s4wave_flowgraph

import "github.com/pkg/errors"

// Validate checks the node type and unique, directed ports.
func (n *FlowgraphNode) Validate() error {
	// Require a type and Step instructions that agree with that type.
	if n.GetTypeId() == "" {
		return errors.New("node type_id is required")
	}
	if (n.GetTypeId() == StepNodeTypeID) != (n.GetStep() != nil) {
		return errors.New("step instructions are required only for a step node")
	}

	// Require one identity and a data contract for each port.
	seen := make(map[string]struct{}, len(n.GetPorts()))
	for _, port := range n.GetPorts() {
		if !validID(port.GetName()) || port.GetTypeId() == "" {
			return errors.New("port name and type_id are required")
		}
		if _, exists := seen[port.GetName()]; exists {
			return errors.Errorf("duplicate port %q", port.GetName())
		}
		if n.GetStep() != nil && port.GetName() == SpendOutputName {
			return errors.Errorf("step port name %q is reserved", SpendOutputName)
		}
		seen[port.GetName()] = struct{}{}
		switch port.GetDirection() {
		case FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT:
			if port.GetCondition() != "" {
				return errors.New("only Step outputs may have conditions")
			}
		case FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT:
			if n.GetStep() == nil && port.GetCondition() != "" {
				return errors.New("only Step outputs may have conditions")
			}
		default:
			return errors.New("port direction is required")
		}
	}
	return nil
}

// FindPort returns a port by name, or nil when it is absent.
func (n *FlowgraphNode) FindPort(name string) *FlowgraphPort {
	for _, port := range n.GetPorts() {
		if port.GetName() == name {
			return port
		}
	}
	return nil
}
