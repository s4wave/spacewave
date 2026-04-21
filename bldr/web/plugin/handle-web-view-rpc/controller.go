package bldr_web_plugin_handle_web_view_rpc

import (
	"context"
	fmt "fmt"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/blang/semver/v4"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_web_plugin "github.com/s4wave/spacewave/bldr/web/plugin"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/web/plugin/handle-web-view-rpc"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller calls the web plugin to enable forwarding web rpcs to the handler plugin.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// conf is the config
	conf *Config
}

// NewController constructs a new controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	conf *Config,
) *Controller {
	return &Controller{
		le:   le,
		bus:  bus,
		conf: conf,
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		fmt.Sprintf("configures plugin %s to handle web views via %s", c.conf.GetWebPluginId(), c.conf.GetHandlePluginId()),
	)
}

// Execute executes the controller.
// Returning nil ends execution.
func (c *Controller) Execute(ctx context.Context) (rerr error) {
	// Access the web plugin RPC client.
	return bldr_plugin.ExPluginLoadAccessClient(
		ctx,
		c.bus,
		c.conf.GetWebPluginId(),
		func(ctx context.Context, cc srpc.Client) error {
			// Call the RPC service to start forwarding web view requests.
			client := bldr_web_plugin.NewSRPCWebPluginClient(cc)
			call, err := client.HandleWebViewViaPlugin(ctx, c.conf.ToRequest())
			if err != nil {
				return err
			}
			for {
				rsp, err := call.Recv()
				if err != nil {
					return err
				}
				switch b := rsp.GetBody().(type) {
				case *bldr_web_plugin.HandleWebViewViaPluginResponse_Ready:
					if b.Ready {
						c.le.Debugf("web plugin: forwarding web views to plugin %s is ready", c.conf.GetHandlePluginId())
					} else {
						c.le.Debugf("web plugin: forwarding web views to plugin %s is not ready", c.conf.GetHandlePluginId())
					}
				}
			}
		},
	)
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
