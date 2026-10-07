package flowgraph_nodetype

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// ControllerID identifies the core node type controller.
const ControllerID = "spacewave/flowgraph/nodetype"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// Controller resolves LookupFlowgraphNodeType for the node types core
// provides: TCP Port, Local Port and Checkout Root.
type Controller struct {
	// types contains the core node types by type ID.
	types map[string]s4wave_flowgraph.FlowgraphNodeType
}

// NewController constructs the core node type controller.
func NewController() *Controller {
	return &Controller{
		types: map[string]s4wave_flowgraph.FlowgraphNodeType{
			s4wave_flowgraph.TCPPortNodeTypeID:      tcpPort{},
			s4wave_flowgraph.LocalPortNodeTypeID:    localPort{},
			s4wave_flowgraph.CheckoutRootNodeTypeID: checkoutRoot{},
		},
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "core Flowgraph node types")
}

// Execute returns immediately, since the controller only handles directives.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective resolves a LookupFlowgraphNodeType for a core node type.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Resolve only lookups for a core node type.
	dir, ok := di.GetDirective().(s4wave_flowgraph.LookupFlowgraphNodeType)
	if !ok {
		return nil, nil
	}
	nodeType, found := c.types[dir.LookupFlowgraphNodeTypeID()]
	if !found {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]s4wave_flowgraph.LookupFlowgraphNodeTypeValue{nodeType}), nil)
}

// Close releases the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)
