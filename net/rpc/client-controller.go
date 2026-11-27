package bifrost_rpc

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/sirupsen/logrus"
)

// ClientController wraps a srpc.Client and serves LookupRpcClient requests.
type ClientController struct {
	// le is the root logger.
	le *logrus.Entry
	// bus is the controller bus.
	bus bus.Bus
	// info is the controller info.
	info *controller.Info
	// client is the prefix client.
	client *srpc.PrefixClient
	// baseClient is the base client (not prefixed).
	baseClient srpc.Client
	// matchServicePrefixes is the list of service id prefixes to match.
	// strips the prefix before calling invoke
	// if empty, forwards all services
	matchServicePrefixes []string
}

// NewClientController constructs a new controller.
func NewClientController(
	le *logrus.Entry,
	bus bus.Bus,
	info *controller.Info,
	client srpc.Client,
	matchServicePrefixes []string,
) *ClientController {
	return &ClientController{
		le:                   le,
		bus:                  bus,
		info:                 info,
		client:               srpc.NewPrefixClient(client, matchServicePrefixes),
		baseClient:           client,
		matchServicePrefixes: matchServicePrefixes,
	}
}

// GetControllerInfo returns information about the controller.
func (c *ClientController) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// ExecCall calls the remote service without stripping its service ID.
// Service prefixes apply only to clients resolved through LookupRpcClient.
func (c *ClientController) ExecCall(ctx context.Context, service, method string, in, out srpc.Message) error {
	return c.baseClient.ExecCall(ctx, service, method, in, out)
}

// NewStream opens a remote stream without stripping its service ID.
func (c *ClientController) NewStream(ctx context.Context, service, method string, firstMsg srpc.Message) (srpc.Stream, error) {
	return c.baseClient.NewStream(ctx, service, method, firstMsg)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *ClientController) Execute(context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *ClientController) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case LookupRpcClient:
		if len(c.matchServicePrefixes) != 0 {
			_, matchedPrefix := srpc.CheckStripPrefix(d.LookupRpcServiceID(), c.matchServicePrefixes)
			if len(matchedPrefix) == 0 {
				return nil, nil
			}
		}
		return directive.R(directive.NewValueResolver([]LookupRpcClientValue{c.client}), nil)
	}
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *ClientController) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller = (*ClientController)(nil)
	_ srpc.Client           = (*ClientController)(nil)
)
