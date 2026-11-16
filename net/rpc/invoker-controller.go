package bifrost_rpc

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// InvokerController wraps a srpc.Invoker and serves LookupRpcService requests.
type InvokerController struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// info is the controller info
	info *controller.Info
	// invoker serves methods after the matched service prefix is stripped.
	invoker srpc.Invoker
	// matchServicePrefixes is the list of service id prefixes to match.
	// strips the prefix before calling invoke
	// if empty, forwards all services
	matchServicePrefixes []string
}

// NewInvokerController constructs a new controller. Register a queryable
// invoker's handlers before attaching the controller to the bus.
func NewInvokerController(
	le *logrus.Entry,
	bus bus.Bus,
	info *controller.Info,
	invoker srpc.Invoker,
	matchServicePrefixes []string,
) *InvokerController {
	return &InvokerController{
		le:                   le,
		bus:                  bus,
		info:                 info,
		invoker:              invoker,
		matchServicePrefixes: matchServicePrefixes,
	}
}

// GetControllerInfo returns information about the controller.
func (c *InvokerController) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *InvokerController) Execute(rctx context.Context) (rerr error) {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *InvokerController) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case LookupRpcService:
		// Match the service in the invoker's own namespace.
		serviceID := d.LookupRpcServiceID()
		if len(c.matchServicePrefixes) != 0 {
			stripped, matchedPrefix := srpc.CheckStripPrefix(serviceID, c.matchServicePrefixes)
			if len(matchedPrefix) == 0 {
				return nil, nil
			}
			serviceID = stripped
		}

		// An empty namespace dispatches by method; otherwise require the service.
		if queryable, ok := c.invoker.(srpc.QueryableInvoker); serviceID != "" && ok && !queryable.HasService(serviceID) {
			return nil, nil
		}

		// Publish only a service the wrapped invoker can handle.
		return directive.R(NewLookupRpcServiceResolver(c), nil)
	}
	return nil, nil
}

// InvokeMethod invokes the method matching the service & method ID.
// Returns false, nil if not found.
// If service string is empty, ignore it.
func (c *InvokerController) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Route a prefixed service into the wrapped invoker's namespace.
	if len(c.matchServicePrefixes) != 0 {
		stripped, matchedPrefix := srpc.CheckStripPrefix(serviceID, c.matchServicePrefixes)
		if len(matchedPrefix) == 0 {
			return false, nil
		}
		serviceID = stripped
	}

	// Invoke the same underlying service that the controller advertised.
	return c.invoker.InvokeMethod(serviceID, methodID, strm)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *InvokerController) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller = (*InvokerController)(nil)
	_ srpc.Invoker          = (*InvokerController)(nil)
)
