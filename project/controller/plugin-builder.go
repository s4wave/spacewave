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
	"github.com/aperturerobotics/controllerbus/util/keyed"
	"github.com/cenkalti/backoff"
	"github.com/pkg/errors"
)

// pluginBuilderTracker tracks a running plugin build controller.
type pluginBuilderTracker struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin id
	pluginID string
}

// newPluginBuilderTracker constructs a new plugin build controller tracker.
func (c *Controller) newPluginBuilderTracker(key string) (keyed.Routine, *pluginBuilderTracker) {
	tr := &pluginBuilderTracker{
		c:        c,
		pluginID: key,
	}
	return tr.execute, tr
}

// execute executes the tracker.
func (t *pluginBuilderTracker) execute(ctx context.Context) error {
	le := t.c.le.WithField("plugin-id", t.pluginID)
	pluginID, projConf := t.pluginID, t.c.c.GetProjectConfig()
	pluginConfigs := projConf.GetPlugins()
	pluginConfig := pluginConfigs[pluginID]
	if pluginConfig.GetId() == "" {
		le.Debug("no builder configured for this plugin id")
		return nil
	}

	le.Debugf("starting plugin build controller: %s", pluginID)
	conf, err := pluginConfig.Resolve(ctx, t.c.bus)
	if err != nil {
		return err
	}

	// cast to a plugin_builder config
	pconf, ok := conf.GetConfig().(plugin_builder.Config)
	if !ok {
		return errors.Errorf(
			"builder config must implement plugin_builder.Config interface: %s",
			conf.GetConfig().GetConfigID(),
		)
	}

	// set config fields
	pconf.SetPluginId(pluginID)
	t.c.c.CopyToPluginBuilder(pconf)

	pluginWorkingPath := path.Join(t.c.c.GetWorkingPath(), "build", pluginID)
	pconf.SetWorkingPath(pluginWorkingPath)

	// set a slower backoff config
	execBackoff := func() backoff.BackOff {
		ebo := backoff.NewExponentialBackOff()
		ebo.InitialInterval = time.Second
		ebo.Multiplier = 1.5
		ebo.MaxInterval = time.Second * 10
		// ebo.MaxElapsedTime = time.Minute
		return ebo
	}

	nctx, nctxCancel := context.WithCancel(ctx)
	defer nctxCancel()

	var wasDisposed atomic.Bool
	_, _, ctrlRef, err := loader.WaitExecControllerRunning(
		nctx,
		t.c.bus,
		resolver.NewLoadControllerWithConfigAndOpts(pconf, directive.ValueOptions{}, execBackoff),
		func() {
			wasDisposed.Store(true)
			nctxCancel()
		},
	)
	if err != nil {
		return err
	}
	defer ctrlRef.Release()

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
