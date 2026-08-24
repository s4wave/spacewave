package plugin_entrypoint_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/retry"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
)

// resolveLoadPlugin resolves a LoadPlugin directive.
func (c *Controller) resolveLoadPlugin(
	dir bldr_plugin.LoadPlugin,
) (directive.Resolver, error) {
	return &loadPluginResolver{
		c:                c,
		pluginID:         dir.LoadPluginID(),
		instanceKey:      dir.LoadPluginInstanceKey(),
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		pluginLoadStateCtr: ccontainer.NewCContainer[bldr_plugin.PluginLoadState](
			bldr_plugin.NewPluginLoadState(
				nil,
				bldr_plugin.InitialCapabilityRegistrationPending,
			),
		),
		bo: buildBackoff(),
	}, nil
}

// loadPluginResolver resolves LoadPlugin with the controller.
type loadPluginResolver struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin identifier
	pluginID string
	// instanceKey is the plugin instance key
	instanceKey string
	// runningPluginCtr contains the running plugin when the plugin is running
	// nil otherwise
	runningPluginCtr *ccontainer.CContainer[bldr_plugin.RunningPlugin]
	// pluginLoadStateCtr atomically tracks the remote plugin's readiness.
	pluginLoadStateCtr *ccontainer.CContainer[bldr_plugin.PluginLoadState]
	// bo is the backoff
	bo cbackoff.BackOff
}

// Resolve resolves the values, emitting them to the handler.
func (r *loadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	le := r.c.le.WithField("load-plugin-id", r.pluginID)
	if r.instanceKey != "" {
		le = le.WithField("instance-key", r.instanceKey)
	}
	le.Debug("loading plugin via plugin host")

	return retry.Retry(ctx, le, func(ctx context.Context, success func()) error {
		r.runningPluginCtr.SetValue(nil)
		r.pluginLoadStateCtr.SetValue(bldr_plugin.NewPluginLoadState(
			nil,
			bldr_plugin.InitialCapabilityRegistrationPending,
		))
		_ = handler.ClearValues()

		strm, err := r.c.srv.LoadPlugin(ctx, &bldr_plugin.LoadPluginRequest{
			PluginId:    r.pluginID,
			InstanceKey: r.instanceKey,
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
				r.pluginLoadStateCtr.SetValue(bldr_plugin.NewPluginLoadState(
					nil,
					bldr_plugin.InitialCapabilityRegistrationPending,
				))
				_ = handler.ClearValues()
				continue
			}

			// construct the rpc stream client
			le.Debug("plugin loaded")
			rpcClient := r.c.BuildRemotePluginClient(r.pluginID, r.instanceKey, false)
			val := bldr_plugin.NewRunningPlugin(rpcClient)
			r.runningPluginCtr.SetValue(val)
			r.pluginLoadStateCtr.SetValue(bldr_plugin.NewPluginLoadState(
				rpcClient,
				bldr_plugin.InitialCapabilityRegistrationComplete,
			))
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

// GetPluginLoadStateCtr returns the atomic RPC-client and initial
// capability-registration state for the remote plugin.
func (r *loadPluginResolver) GetPluginLoadStateCtr() ccontainer.Watchable[bldr_plugin.PluginLoadState] {
	return r.pluginLoadStateCtr
}

// _ is a type assertion
var (
	_ directive.Resolver           = (*loadPluginResolver)(nil)
	_ bldr_plugin.RunningPluginRef = (*loadPluginResolver)(nil)
)
