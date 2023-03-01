package bldr_project_controller

import (
	"context"
	"path"
	"sync/atomic"
	"time"

	plugin_builder "github.com/aperturerobotics/bldr/plugin/builder"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
)

// pluginBuilderTracker tracks a running plugin build controller.
type pluginBuilderTracker struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
	// builderCtrlPromise is the promise for the plugin builder controller.
	builderCtrlPromise *promise.PromiseContainer[plugin_builder.Controller]
}

// newPluginBuilderTracker constructs a new plugin build controller tracker.
func (c *Controller) newPluginBuilderTracker(key string) (keyed.Routine, *pluginBuilderTracker) {
	tr := &pluginBuilderTracker{
		c:                  c,
		pluginID:           key,
		builderCtrlPromise: promise.NewPromiseContainer[plugin_builder.Controller](),
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *pluginBuilderTracker) execute(ctx context.Context) error {
	le := t.c.le.WithField("plugin-id", t.pluginID)
	pluginID, projConf := t.pluginID, t.c.c.GetProjectConfig()
	pluginConfigs := projConf.GetPlugins()
	pluginConfig := pluginConfigs[pluginID]
	builderPromise := promise.NewPromise[plugin_builder.Controller]()
	t.builderCtrlPromise.SetPromise(builderPromise)
	if pluginConfig.GetId() == "" {
		err := errors.New("no builder configured for this plugin id")
		le.Warn(err.Error())
		builderPromise.SetResult(nil, errors.Wrap(err, t.pluginID))
		return nil
	}

	le.Debugf("starting plugin build controller: %s", pluginID)
	conf, err := pluginConfig.Resolve(ctx, t.c.bus)
	if err != nil {
		builderPromise.SetResult(nil, err)
		return err
	}

	// cast to a plugin_builder config
	pconf, ok := conf.GetConfig().(plugin_builder.ControllerConfig)
	if !ok {
		err := errors.Errorf(
			"config must implement plugin_builder.ControllerConfig interface: %s",
			conf.GetConfig().GetConfigID(),
		)
		builderPromise.SetResult(nil, err)
		return err
	}

	// set config fields
	pluginWorkingPath := path.Join(t.c.c.GetWorkingPath(), "plugin", "build", pluginID)
	pconf.SetPluginBuilderConfig(t.c.c.ToPluginBuilderConfig(
		pluginID,
		pluginWorkingPath,
	))
	if t.c.c.GetDisableWatch() {
		pconf.SetDisableWatch(true)
	}

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
		t.c.bus,
		resolver.NewLoadControllerWithConfigAndOpts(pconf, directive.ValueOptions{}, execBackoff),
		func() {
			wasDisposed.Store(true)
			nctxCancel()
		},
	)
	if err != nil {
		builderPromise.SetResult(nil, err)
		return err
	}
	defer ctrlRef.Release()

	builderCtrl, ok := builderCtrlInter.(plugin_builder.Controller)
	if !ok {
		err := errors.Errorf("type must implement plugin_builder.Controller: %#v", builderCtrlInter)
		builderPromise.SetResult(nil, err)
		return err
	}
	builderPromise.SetResult(builderCtrl, nil)

	select {
	case <-ctx.Done():
		return context.Canceled
	case <-nctx.Done():
	}

	if wasDisposed.Load() {
		return errors.Wrap(err, "directive disposed unexpectedly")
	}

	return context.Canceled
}
