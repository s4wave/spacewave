package app

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver"
)

// ControllerID is the controller id.
const ControllerID = "app"

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "app controller"

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
	le.Info("app running")
	defer le.Info("app closed")

	for {
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-time.After(time.Second):
		}

		le.Infof("app is running: %v", time.Now().String())
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (d *App) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// _ is a type assertion
var _ controller.Controller = (*App)(nil)
