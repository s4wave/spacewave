package plugin_entrypoint_controller

import (
	"context"

	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/retry"
	cbackoff "github.com/cenkalti/backoff"
)

// resolveLoadPlugin resolves a LoadPlugin directive.
func (c *Controller) resolveLoadPlugin(
	ctx context.Context,
	di directive.Instance,
	dir bldr_plugin.LoadPlugin,
) (directive.Resolver, error) {
	pluginID := dir.LoadPluginID()
	return &loadPluginResolver{
		c:            c,
		pluginID:     pluginID,
		rpcClientCtr: ccontainer.NewCContainer[*srpc.Client](nil),
		bo:           buildBackoff(),
	}, nil
}

// loadPluginResolver resolves LoadPlugin with the controller.
type loadPluginResolver struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin identifier
	pluginID string
	// rpcClientCtr is the rpc client container
	rpcClientCtr *ccontainer.CContainer[*srpc.Client]
	// bo is the backoff
	bo cbackoff.BackOff
}

// Resolve resolves the values, emitting them to the handler.
func (r *loadPluginResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	le := r.c.le.WithField("load-plugin-id", r.pluginID)
	if handler.CountValues(false) == 0 {
		var resultValue bldr_plugin.LoadPluginValue = r
		_, _ = handler.AddValue(resultValue)
	}

	le.Debug("loading plugin via plugin host")
	return retry.Retry(ctx, le, func(ctx context.Context, success func()) error {
		r.rpcClientCtr.SetValue(nil)
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
				return err
			}

			nextRunning := resp.GetPluginStatus().GetRunning()
			if nextRunning == running {
				continue
			}
			running = nextRunning

			if !running {
				le.Debug("plugin not yet loaded")
				r.rpcClientCtr.SetValue(nil)
				continue
			}

			// construct the rpc stream client
			le.Debug("plugin loaded")
			rpcClient := r.c.BuildRemotePluginClient(r.pluginID, false)
			r.rpcClientCtr.SetValue(&rpcClient)
		}
	}, r.bo)
}

// GetRpcClientCtr returns the rpc client container.
// The plugin RPC client will be set when the plugin becomes ready.
func (r *loadPluginResolver) GetRpcClientCtr() *ccontainer.CContainer[*srpc.Client] {
	return r.rpcClientCtr
}

// _ is a type assertion
var (
	_ directive.Resolver        = ((*loadPluginResolver)(nil))
	_ bldr_plugin.RunningPlugin = ((*loadPluginResolver)(nil))
)
