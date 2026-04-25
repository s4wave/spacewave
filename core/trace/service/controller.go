package trace_service

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_trace "github.com/s4wave/spacewave/sdk/trace"
)

// ControllerID is the controller identifier.
const ControllerID = "trace/service"

// Version is the component version.
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
const controllerDescrip = "runtime trace rpc service controller"

// Controller exposes the trace service through LookupRpcService.
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
		controllerDescrip,
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			mux := srpc.NewMux()
			if err := s4wave_trace.SRPCRegisterTraceService(mux, NewService()); err != nil {
				return nil, err
			}
			return &Controller{BusController: base, mux: mux}, nil
		},
	)
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() == s4wave_trace.SRPCTraceServiceServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c), nil)
		}
	}

	return nil, nil
}

// InvokeMethod invokes the method matching the service and method IDs.
func (c *Controller) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	return c.mux.InvokeMethod(serviceID, methodID, strm)
}

// _ is a type assertion
var (
	_ controller.Controller = (*Controller)(nil)
	_ srpc.Invoker          = (*Controller)(nil)
)
