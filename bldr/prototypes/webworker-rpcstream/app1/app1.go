package app1

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	prototype_webworker_rpcstream_common "github.com/s4wave/spacewave/bldr/prototypes/webworker-rpcstream/common"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// ControllerID is the controller id.
const ControllerID = "app1"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "app1 controller"

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
			_ = echo.SRPCRegisterEchoer(mux, echo.NewEchoServer(nil))
			return &App{BusController: base, mux: mux}, nil
		},
	)
}

// Execute executes the controller goroutine.
func (d *App) Execute(ctx context.Context) error {
	le := d.GetLogger()
	le.Info("app1 running")

	// Get a handle to app2.
	le.Info("app1 loading app2")
	plug, _, plugRef, err := bldr_plugin.ExLoadPlugin(ctx, d.GetBus(), false, "app2", nil)
	if err != nil {
		return err
	}
	defer plugRef.Release()

	le.Info("app1 starting request to app2")
	plugRpcClient := plug.GetRpcClient()
	rpcClient := prototype_webworker_rpcstream_common.NewSRPCPrototypeServiceClient(plugRpcClient)

	testBody := "hello from app1"
	strm, err := rpcClient.Prototype(ctx, &prototype_webworker_rpcstream_common.PrototypeRequest{Body: testBody})
	if err != nil {
		return err
	}

	le.Info("started Prototype rpc with app2")
	waitTimer := time.After(time.Second * 5)
WaitLoop:
	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitTimer:
			break WaitLoop
		default:
		}

		resp, err := strm.Recv()
		if err != nil {
			return err
		}
		le.Infof("got response from app2: %v", resp.String())
	}

	le.Info("closing stream")
	if err := strm.CloseSend(); err != nil {
		return err
	}
	if err := strm.Close(); err != nil {
		return err
	}

	le.Info("stream closed")

	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (d *App) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case bifrost_rpc.LookupRpcService:
		if dir.LookupRpcServiceID() == echo.SRPCEchoerServiceID {
			return directive.R(bifrost_rpc.NewLookupRpcServiceResolver(d.mux), nil)
		}
	}

	return nil, nil
}

// _ is a type assertion
var _ controller.Controller = (*App)(nil)
