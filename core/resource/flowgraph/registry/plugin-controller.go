package resource_flowgraph_registry

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	flowgraph_registry "github.com/s4wave/spacewave/sdk/flowgraph/registry"
)

// pluginControllerVersion is the version of the plugin controller proxies.
var pluginControllerVersion = controller.MustParseVersion("0.0.1")

// pluginConfig carries a plugin config's encoded bytes through the host bus,
// which has no Go type for the config, bound to the registration whose plugin
// compiled the config. The plugin decodes the bytes.
type pluginConfig struct {
	// configID is the plugin config ID.
	configID string
	// data is the config encoded as protobuf or JSON.
	data []byte
	// reg is the registration whose plugin runs the config.
	reg *nodeTypeRegistration
}

// GetConfigID returns the plugin config ID.
func (c *pluginConfig) GetConfigID() string {
	return c.configID
}

// Validate accepts any bytes because the plugin validates the decoded config.
func (c *pluginConfig) Validate() error {
	return nil
}

// EqualsConfig compares the config ID, registration and encoded bytes.
func (c *pluginConfig) EqualsConfig(other config.Config) bool {
	oc, ok := other.(*pluginConfig)
	return ok && c.configID == oc.configID && c.reg == oc.reg && bytes.Equal(c.data, oc.data)
}

// MarshalJSON returns the bytes when they are JSON.
func (c *pluginConfig) MarshalJSON() ([]byte, error) {
	if len(c.data) == 0 {
		return []byte("{}"), nil
	}
	if c.data[0] != '{' {
		return nil, errors.Errorf("plugin config %q is binary", c.configID)
	}
	return bytes.Clone(c.data), nil
}

// UnmarshalJSON keeps the JSON bytes.
func (c *pluginConfig) UnmarshalJSON(data []byte) error {
	c.data = bytes.Clone(data)
	return nil
}

// SizeVT returns the encoded size.
func (c *pluginConfig) SizeVT() int {
	return len(c.data)
}

// MarshalVT returns a copy of the encoded bytes.
func (c *pluginConfig) MarshalVT() ([]byte, error) {
	return bytes.Clone(c.data), nil
}

// MarshalToSizedBufferVT writes the encoded bytes to the end of buf.
func (c *pluginConfig) MarshalToSizedBufferVT(buf []byte) (int, error) {
	return copy(buf[len(buf)-len(c.data):], c.data), nil
}

// UnmarshalVT keeps the protobuf bytes.
func (c *pluginConfig) UnmarshalVT(data []byte) error {
	c.data = bytes.Clone(data)
	return nil
}

// Reset clears the encoded bytes.
func (c *pluginConfig) Reset() {
	c.data = nil
}

// pluginFactory builds controllers that run one plugin config inside the
// plugin, for one registration.
type pluginFactory struct {
	// configID is the plugin config ID.
	configID string
	// reg is the registration whose plugin runs the controllers.
	reg *nodeTypeRegistration
}

// GetConfigID returns the plugin config ID.
func (f *pluginFactory) GetConfigID() string {
	return f.configID
}

// ConstructConfig returns an empty config for the ID bound to the registration.
func (f *pluginFactory) ConstructConfig() config.Config {
	return &pluginConfig{configID: f.configID, reg: f.reg}
}

// GetVersion returns the proxy version.
func (f *pluginFactory) GetVersion() controller.Version {
	return pluginControllerVersion
}

// Construct starts the controller in the plugin and returns once the plugin
// reports it running, so the host shows the controller only while it runs.
func (f *pluginFactory) Construct(ctx context.Context, conf config.Config, _ controller.ConstructOpts) (controller.Controller, error) {
	// Accept only this factory's config.
	pc, ok := conf.(*pluginConfig)
	if !ok || pc.configID != f.configID || pc.reg != f.reg {
		return nil, errors.Errorf("plugin factory %q got config %q", f.configID, conf.GetConfigID())
	}

	// Run the config for as long as the controller and the registration live.
	strmCtx, cancel := context.WithCancel(f.reg.ctx)
	stop := context.AfterFunc(ctx, cancel)
	defer stop()
	client := flowgraph_registry.NewSRPCNodeTypeResourceServiceClient(f.reg.client)
	strm, err := client.ExecController(strmCtx, &flowgraph_registry.ExecControllerRequest{
		Config: &configset_proto.ControllerConfig{Id: pc.configID, Config: bytes.Clone(pc.data)},
	})
	if err != nil {
		cancel()
		return nil, err
	}

	// Wait for the plugin to run the controller.
	// The stream's error explains a failure.
	for {
		resp, err := strm.Recv()
		if err != nil {
			cancel()
			return nil, errors.Wrap(err, "run in plugin")
		}
		if resp.GetStatus() == controller_exec.ControllerStatus_ControllerStatus_RUNNING {
			return &pluginController{configID: pc.configID, strm: strm, cancel: cancel}, nil
		}
	}
}

// pluginController holds a controller running inside a plugin.
type pluginController struct {
	// configID is the plugin config ID.
	configID string
	// strm streams the plugin controller's status.
	strm flowgraph_registry.SRPCNodeTypeResourceService_ExecControllerClient
	// cancel ends the stream, which stops the plugin controller.
	cancel context.CancelFunc
}

// GetControllerInfo describes the proxy.
func (c *pluginController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("resource/flowgraph-plugin/"+c.configID, pluginControllerVersion, "controller running in a plugin")
}

// Execute follows the plugin controller until it stops running or ctx ends.
func (c *pluginController) Execute(ctx context.Context) error {
	// End the stream when the host removes the controller.
	stop := context.AfterFunc(ctx, c.cancel)
	defer stop()

	// Fail when the plugin controller leaves the running state, so the host
	// restarts it and shows it starting meanwhile.
	for {
		resp, err := c.strm.Recv()
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "run in plugin")
		}
		switch resp.GetStatus() {
		case controller_exec.ControllerStatus_ControllerStatus_RUNNING, controller_exec.ControllerStatus_ControllerStatus_ERROR:
		default:
			return errors.New("plugin controller stopped running")
		}
	}
}

// HandleDirective resolves nothing. The plugin controller resolves on its bus.
func (c *pluginController) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close stops the plugin controller.
func (c *pluginController) Close() error {
	c.cancel()
	return nil
}

// _ is a type assertion.
var (
	_ config.Config      = (*pluginConfig)(nil)
	_ controller.Factory = (*pluginFactory)(nil)
)
