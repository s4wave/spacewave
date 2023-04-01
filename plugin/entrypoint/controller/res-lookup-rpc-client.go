package plugin_entrypoint_controller

import (
	"context"
	"strings"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
)

// resolveLookupRpcClient resolves a LookupRpcClient directive.
func (c *Controller) resolveLookupRpcClient(
	ctx context.Context,
	di directive.Instance,
	dir bifrost_rpc.LookupRpcClient,
) (directive.Resolver, error) {
	serviceID := dir.LookupRpcServiceID()

	// check if the service ID matches one of the known prefixes.
	matchedService, matchedPrefix := srpc.CheckStripPrefix(serviceID, []string{
		bldr_plugin.PluginServiceIDPrefix,
		bldr_plugin.HostServiceIDPrefix,
	})
	if len(matchedPrefix) == 0 {
		return nil, nil
	}

	if matchedPrefix == bldr_plugin.PluginServiceIDPrefix {
		pluginID, remoteServiceID, ok := strings.Cut(matchedService, "/")
		if !ok || remoteServiceID == "" {
			// ignore: we require the following format:
			// plugin/{plugin-id}/{service-id}
			return nil, nil
		}
		if err := bldr_plugin.ValidatePluginID(pluginID, false); err != nil {
			// ignore it: invalid plugin id
			return nil, nil
		}
		// call via the plugin
		return &resolveLookupRpcClientViaPlugin{
			c:                    c,
			pluginID:             pluginID,
			rpcClientCtr:         ccontainer.NewCContainer[*srpc.Client](nil),
			stripServiceIDPrefix: serviceID[:len(bldr_plugin.PluginServiceIDPrefix)+len(pluginID)+1],
		}, nil
	}

	return bifrost_rpc.NewLookupRpcClientResolver(c.hostPrefixClient), nil
}

// resolveLookupRpcClientViaPlugin resolves LookupRpcClient via accessing a plugin client.
type resolveLookupRpcClientViaPlugin struct {
	// c is the controller
	c *Controller
	// pluginID is the plugin identifier
	pluginID string
	// rpcClientCtr is the rpc client container
	rpcClientCtr *ccontainer.CContainer[*srpc.Client]
	// stripServiceIDPrefix is the prefix to strip from the service id
	stripServiceIDPrefix string
}

// GetRpcClientCtr returns the rpc client container.
// The plugin RPC client will be set when the plugin becomes ready.
func (r *resolveLookupRpcClientViaPlugin) GetRpcClientCtr() *ccontainer.CContainer[*srpc.Client] {
	return r.rpcClientCtr
}

// Resolve resolves the values, emitting them to the handler.
func (r *resolveLookupRpcClientViaPlugin) Resolve(rctx context.Context, handler directive.ResolverHandler) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()
	r.rpcClientCtr.SetValue(nil)

	client, ref, err := bldr_plugin.ExPluginLoadWaitClient(ctx, r.c.b, r.pluginID, ctxCancel)
	if err != nil {
		return err
	}

	prefixClient := srpc.NewPrefixClient(client, []string{r.stripServiceIDPrefix})
	var value bifrost_rpc.LookupRpcClientValue = prefixClient
	r.rpcClientCtr.SetValue(&value)
	_, _ = handler.AddValue(value)
	handler.MarkIdle()

	<-ctx.Done()
	ref.Release()
	handler.ClearValues()
	return context.Canceled
}

// _ is a type assertion
var _ directive.Resolver = ((*resolveLookupRpcClientViaPlugin)(nil))
