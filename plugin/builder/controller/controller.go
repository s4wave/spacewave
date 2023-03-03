package bldr_plugin_builder_controller

import (
	"context"
	"sync/atomic"
	"time"

	plugin_builder "github.com/aperturerobotics/bldr/plugin/builder"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/promise"
	"github.com/blang/semver"
	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bldr/plugin/builder/controller"

// Controller is the plugin builder controller.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// c is the controller config
	c *Config
	// resultPromise contains the result of the compilation.
	resultPromise *promise.PromiseContainer[*plugin_builder.PluginBuilderResult]
}

// NewController constructs a new controller.
func NewController(le *logrus.Entry, bus bus.Bus, cc *Config) *Controller {
	return &Controller{
		le:            le,
		bus:           bus,
		c:             cc,
		resultPromise: promise.NewPromiseContainer[*plugin_builder.PluginBuilderResult](),
	}
}

// GetConfig returns the config.
func (c *Controller) GetConfig() *Config {
	return c.c
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"bldr plugin builder: plugin "+c.c.GetBuilderConfig().GetPluginManifestMeta().GetPluginId(),
	)
}

// GetResultPromise returns the result promise.
func (c *Controller) GetResultPromise() promise.PromiseLike[*plugin_builder.PluginBuilderResult] {
	return c.resultPromise
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	c.resultPromise.SetPromise(nil)
	builderConfig := c.GetConfig().GetBuilderConfig()
	meta := builderConfig.GetPluginManifestMeta()
	pluginID := meta.GetPluginId()
	le := c.le.WithField("plugin-id", pluginID)
	pluginConfig := c.GetConfig().GetBuilderControllerConfig()

	le.Debugf("starting plugin build controller: %s", pluginID)
	conf, err := pluginConfig.Resolve(ctx, c.bus)
	if err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// cast to a plugin_builder config
	pconf, ok := conf.GetConfig().(plugin_builder.ControllerConfig)
	if !ok {
		err := errors.Errorf(
			"config must implement plugin_builder.ControllerConfig interface: %s",
			conf.GetConfig().GetConfigID(),
		)
		c.resultPromise.SetResult(nil, err)
		return err
	}

	// set config fields
	pconf.SetPluginBuilderConfig(builderConfig)

	// set build backoff config
	execBackoff := func() backoff.BackOff {
		ebo := backoff.NewExponentialBackOff()
		ebo.InitialInterval = time.Second
		ebo.Multiplier = 2
		ebo.MaxInterval = time.Second * 10
		// ebo.MaxElapsedTime = time.Minute
		return ebo
	}

	nctx, nctxCancel := context.WithCancel(ctx)
	defer nctxCancel()

	var wasDisposed atomic.Bool
	builderCtrlInter, _, ctrlRef, err := loader.WaitExecControllerRunning(
		nctx,
		c.bus,
		resolver.NewLoadControllerWithConfigAndOpts(pconf, directive.ValueOptions{}, execBackoff),
		func() {
			wasDisposed.Store(true)
			nctxCancel()
		},
	)
	if err != nil {
		c.resultPromise.SetResult(nil, err)
		return err
	}
	defer ctrlRef.Release()

	builderCtrl, ok := builderCtrlInter.(plugin_builder.Controller)
	if !ok {
		err := errors.Errorf("type must implement plugin_builder.Controller: %#v", builderCtrlInter)
		c.resultPromise.SetResult(nil, err)
		return err
	}

	resultPromise := builderCtrl.GetResultPromise()
	c.resultPromise.SetPromise(resultPromise)
	_, err = resultPromise.Await(ctx)
	return err
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case plugin_host.FetchPlugin:
		return directive.R(c.resolveFetchPlugin(ctx, di, d), nil)
	}
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
