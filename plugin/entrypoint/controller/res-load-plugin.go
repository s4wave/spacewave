package plugin_entrypoint_controller

import (
	"context"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/retry"
	cbackoff "github.com/cenkalti/backoff/v4"
)

// resolveLoadPlugin resolves a LoadPlugin directive.
func (c *Controller) resolveLoadPlugin(
	ctx context.Context,
	di directive.Instance,
	dir bldr_plugin.LoadPlugin,
) (directive.Resolver, error) {
	return &loadPluginResolver{
		c:                c,
		pluginID:         dir.LoadPluginID(),
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		bo:               buildBackoff(),
	}, nil
}

// loadPluginResolver resolves LoadPlugin with the controller.
type loadPluginResolver struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin identifier
	pluginID string
	// runningPluginCtr contains the running plugin when the plugin is running
	// nil otherwise
	runningPluginCtr *ccontainer.CContainer[bldr_plugin.RunningPlugin]
	// bo is the backoff
	bo cbackoff.BackOff
}

// Resolve resolves the values, emitting them to the handler.
func (r *loadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	le := r.c.le.WithField("load-plugin-id", r.pluginID)
	le.Debug("loading plugin via plugin host")

	return retry.Retry(ctx, le, func(ctx context.Context, success func()) error {
		r.runningPluginCtr.SetValue(nil)
		_ = handler.ClearValues()

		strm, err := r.c.srv.LoadPlugin(ctx, &bldr_plugin.LoadPluginRequest{
			PluginId: r.pluginID,
		})
		if err != nil {
			return err
		}
		defer strm.Close()

		var running bool
		for {
			resp, err := strm.Recv()
			if err != nil {
				_ = handler.ClearValues()
				return err
			}

			nextRunning := resp.GetPluginStatus().GetRunning()
			success()
			if nextRunning == running {
				continue
			}
			running = nextRunning

			if !running {
				le.Debug("plugin not yet loaded")
				r.runningPluginCtr.SetValue(nil)
				_ = handler.ClearValues()
				continue
			}

			// construct the rpc stream client
			le.Debug("plugin loaded")
			rpcClient := r.c.BuildRemotePluginClient(r.pluginID, false)
			var val bldr_plugin.LoadPluginValue = bldr_plugin.NewRunningPlugin(rpcClient)
			r.runningPluginCtr.SetValue(val)
			_, _ = handler.AddValue(val)
			handler.MarkIdle(true)
		}
	}, r.bo)
}

// GetRunningPluginCtr returns the current running plugin instance.
// May be changed (or set to nil) when the instance changes.
func (r *loadPluginResolver) GetRunningPluginCtr() ccontainer.Watchable[bldr_plugin.RunningPlugin] {
	return r.runningPluginCtr
}

// _ is a type assertion
var (
	_ directive.Resolver           = ((*loadPluginResolver)(nil))
	_ bldr_plugin.RunningPluginRef = ((*loadPluginResolver)(nil))
)
