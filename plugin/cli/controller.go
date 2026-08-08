package cli_plugin

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_cli_terminal "github.com/s4wave/spacewave/sdk/cli/terminal"
)

// ControllerID is the controller identifier.
const ControllerID = "plugin/cli"

// Version is the component version.
var Version = controller.MustParseVersion("0.0.1")

// Controller serves the browser CLI terminal RPC surface.
type Controller struct {
	*bus.BusController[*Config]

	mux srpc.Mux
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		"spacewave cli browser terminal controller",
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			ctrl := &Controller{BusController: base}
			mux := srpc.NewMux()
			if err := s4wave_cli_terminal.SRPCRegisterCliTerminalService(mux, NewTerminalService(NewCoreClientFactory(b))); err != nil {
				return nil, err
			}
			ctrl.mux = mux
			return ctrl, nil
		},
	)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Inspect the requested RPC service directive.
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		// Resolve the CLI terminal service when its ID matches.
		if d.LookupRpcServiceID() == s4wave_cli_terminal.SRPCCliTerminalServiceServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
	}

	// Leave unsupported RPC services unresolved.
	return nil, nil
}

// _ is a type assertion.
var _ controller.Controller = (*Controller)(nil)
