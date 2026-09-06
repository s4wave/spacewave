package bldr_manifest_materializer

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// ControllerID is the controller identifier.
const ControllerID = "bldr/manifest/materializer"

// PluginID is the manifest id for the materializer plugin.
const PluginID = "bldr-materializer"

// Version is the component version.
var Version = controller.MustParseVersion("0.0.1")

// Controller serves the Materializer service over a local SRPC mux.
type Controller struct {
	*bus.BusController[*Config]

	// mux serves the materializer SRPC service.
	mux srpc.Invoker
}

// NewFactory constructs the component factory.
func NewFactory(b bus.Bus) controller.Factory {
	return bus.NewBusControllerFactory(
		b,
		ConfigID,
		ControllerID,
		Version,
		"bldr manifest materializer service controller",
		func() *Config {
			return &Config{}
		},
		func(base *bus.BusController[*Config]) (*Controller, error) {
			mux := srpc.NewMux()
			if err := SRPCRegisterMaterializer(mux, NewMaterializer(base.GetLogger(), b, transform_all.BuildFactorySet())); err != nil {
				return nil, err
			}
			return &Controller{BusController: base, mux: mux}, nil
		},
	)
}

// HandleDirective resolves the materializer service lookup.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// Resolve the materializer service when its ID matches.
	switch d := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if d.LookupRpcServiceID() == SRPCMaterializerServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(c.mux), nil)
		}
	}

	// Leave unsupported directives unresolved.
	return nil, nil
}

// _ is a type assertion.
var _ controller.Controller = (*Controller)(nil)
