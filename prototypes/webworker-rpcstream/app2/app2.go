package app2

import (
	"context"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	prototype_webworker_rpcstream_common "github.com/aperturerobotics/bldr/prototypes/webworker-rpcstream/common"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
)

// ControllerID is the controller id.
const ControllerID = "app2"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "app2 controller"

// App is the app controller.
type App struct {
	*bus.BusController[*Config]

	// mux is the rpc mux the web view will call
	mux srpc.Mux
}

// NewFactory constructs the controller factory.
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
		func(base *bus.BusController[*Config]) (*App, error) {
			mux := srpc.NewMux()
			app := &App{BusController: base, mux: mux}
			_ = prototype_webworker_rpcstream_common.SRPCRegisterPrototypeService(mux, NewPrototypeHost(app))
			return app, nil
		},
	)
}

// Execute executes the controller goroutine.
func (d *App) Execute(ctx context.Context) error {
	le := d.GetLogger()
	le.Info("app2 running")

	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (d *App) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if dir.LookupRpcServiceID() == prototype_webworker_rpcstream_common.SRPCPrototypeServiceServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(d.mux), nil)
		}
	}

	return nil, nil
}

// _ is a type assertion
var _ controller.Controller = (*App)(nil)
