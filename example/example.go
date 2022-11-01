package bldr_example

import (
	"context"
	"errors"
	"time"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/bifrost/util/backoff"
	"github.com/aperturerobotics/bldr/plugin"
	web_view "github.com/aperturerobotics/bldr/web/view"
	web_view_handler "github.com/aperturerobotics/bldr/web/view/handler"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	kvtx_vlogger "github.com/aperturerobotics/hydra/kvtx/vlogger"
	"github.com/aperturerobotics/hydra/object"
	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller id.
const ControllerID = "bldr/example/demo"

// ExampleScriptPath is the path to the example.tsx script.
//
//bldr:esbuild example.tsx
var ExampleScriptPath string

// Version is the controller version
var Version = semver.MustParse("0.0.1")

// controllerDescrip is the controller description.
var controllerDescrip = "demo controller"

// Demo is a demo controller.
type Demo struct {
	*bus.BusController[*Config]
}

// NewDemo constructs a new demo controller.
func NewDemo(le *logrus.Entry, b bus.Bus, conf *Config) (*Demo, error) {
	return &Demo{
		BusController: bus.NewBusController(
			le,
			b,
			conf,
			ControllerID,
			Version,
			controllerDescrip,
		),
	}, nil
}

// NewFactory constructs the demo controller factory.
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
		func(base *bus.BusController[*Config]) (*Demo, error) {
			return &Demo{BusController: base}, nil
		},
	)
}

// Execute executes the controller goroutine.
func (d *Demo) Execute(ctx context.Context) error {
	b := d.GetBus()
	le := d.GetLogger()

	// Example: call the Echo service to prove the RPC communication is working.
	go func() {
		le.Debug("attempting to lookup Echo() service")
		// TODO: add a srpc.Client which calls LookupRpcClientSet on-demand with refcount per-service
		hostEchoServiceID := plugin.HostServiceIDPrefix + echo.SRPCEchoerServiceID
		echoClientSet, echoClientSetRef, err := bifrost_rpc.ExLookupRpcClientSet(ctx, b, hostEchoServiceID, ControllerID)
		if err != nil {
			le.WithError(err).Warn("unable to lookup rpc client set for echo service")
			return
		}
		defer echoClientSetRef.Release()

		bo := (&backoff.Backoff{
			BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
			Exponential: &backoff.Exponential{
				InitialInterval: 1000,
				MaxInterval:     10000,
				Multiplier:      2,
			},
		}).Construct()
		for {
			le.Debug("attempting to call echo() service on plugin host")
			echoService := echo.NewSRPCEchoerClientWithServiceID(echoClientSet, hostEchoServiceID)
			resp, err := echoService.Echo(ctx, &echo.EchoMsg{
				Body: "hello from plugin: " + time.Now().String(),
			})
			if err != nil {
				le.WithError(err).Warn("error calling echo() service")
			} else {
				le.Debugf("successfully called host echo() service: %s", resp.GetBody())
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(bo.NextBackOff()):
			}
		}
	}()

	le.Info("hello from the bldr example demo controller")
	le.Info("creating LookupVolume directive for the plugin host volume")
	vol, volRef, err := volume.ExLookupVolume(ctx, b, plugin.PluginVolumeID, "", false)
	if err == nil && volRef == nil {
		err = errors.New("lookup host volume returned not found")
	}
	if err != nil {
		le.WithError(err).Warn("failed to lookup host volume")
		return err
	}

	le.Info("successfully looked up volume")
	defer volRef.Release()

	le.Info("testing object store api")
	if err := store_test.TestObjectStore(ctx, vol, func(obj object.ObjectStore) (object.ObjectStore, error) {
		return kvtx_vlogger.NewVLogger(le, obj), nil
	}); err != nil {
		return err
	}

	le.Info("testing message queue api")
	if err := store_test.TestMqueueAPI(ctx, vol); err != nil {
		return err
	}

	le.Info("volume tests passed")
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (d *Demo) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case web_view.HandleWebView:
		return d.resolveHandleWebView(ctx, di, dir)
	}

	return nil, nil
}

// resolveHandleWebView conditionally returns a resolver for a HandleWebView directive.
func (d *Demo) resolveHandleWebView(
	ctx context.Context,
	di directive.Instance,
	dir web_view.HandleWebView,
) ([]directive.Resolver, error) {
	webView := dir.HandleWebView()
	// handle root web views only
	if webView.GetParentId() != "" {
		return nil, nil
	}

	return directive.R(web_view_handler.NewHandleWebViewResolver(
		dir,
		web_view_handler.NewSetReactComponent(ExampleScriptPath, d.GetLogger()),
	), nil)
}

// _ is a type assertion
var _ controller.Controller = (*Demo)(nil)
