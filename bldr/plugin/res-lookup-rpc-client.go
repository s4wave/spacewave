package bldr_plugin

import (
	"context"
	"strings"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
)

// LookupRpcClientHandler handles callbacks for LookupRpcClientResolver.
type LookupRpcClientHandler interface {
	// WaitPluginHostClient waits for an RPC client for the plugin host.
	//
	// Released is a function to call if the client becomes invalid.
	// Returns nil, nil, err if any error.
	// Returns nil, nil, nil to skip resolving the client.
	// Otherwise returns client, releaseFunc, nil. A nil release function means
	// the handler owns the client without a caller reference.
	WaitPluginHostClient(ctx context.Context, released func()) (srpc.Client, func(), error)

	// WaitPluginClient waits for an RPC client for a plugin.
	//
	// Released is a function to call if the client becomes invalid.
	// Returns nil, nil, err if any error.
	// Returns nil, nil, nil to skip resolving the client.
	// Otherwise returns client, releaseFunc, nil. A nil release function means
	// the handler owns the client without a caller reference.
	WaitPluginClient(ctx context.Context, released func(), pluginID string) (srpc.Client, func(), error)
}

// LookupRpcClientResolver resolves LookupRpcClient with the plugin or plugin host.
//
// Resolves service IDs like:
//   - plugin/{plugin-id}/{service id}
//   - plugin-host/{service id}
type LookupRpcClientResolver struct {
	// h supplies retained plugin clients.
	h LookupRpcClientHandler
	// pluginID identifies the plugin, or the host when empty.
	pluginID string
	// rpcClientCtr publishes the current retained RPC client.
	rpcClientCtr *ccontainer.CContainer[*srpc.Client]
	// stripServiceIDPrefix is the prefix to strip from the service ID, if any.
	stripServiceIDPrefix string
}

// NewLookupRpcClientResolver constructs a new LookupRpcClientResolver.
//
// Usually you will want to use ResolveLookupRpcClient instead.
// If pluginID is empty, addresses the plugin host.
// stripServiceIDPrefix is the prefix to strip from the service ID, if any.
func NewLookupRpcClientResolver(h LookupRpcClientHandler, pluginID, stripServiceIDPrefix string) *LookupRpcClientResolver {
	return &LookupRpcClientResolver{
		h:                    h,
		pluginID:             pluginID,
		stripServiceIDPrefix: stripServiceIDPrefix,
		rpcClientCtr:         ccontainer.NewCContainer[*srpc.Client](nil),
	}
}

// matchPluginServiceID parses a service ID against the known plugin service
// prefixes. Returns the plugin id and the service-id prefix to strip, or
// empty strings when no prefix matches.
func matchPluginServiceID(serviceID string) (pluginID, stripPrefix string, ok bool) {
	matchedService, matchedPrefix := srpc.CheckStripPrefix(serviceID, []string{
		PluginServiceIDPrefix,
		HostServiceIDPrefix,
	})
	switch matchedPrefix {
	case PluginServiceIDPrefix:
		id, remoteServiceID, cutOK := strings.Cut(matchedService, "/")
		if !cutOK || remoteServiceID == "" || id == "" {
			// Plugin services require both a plugin ID and a service ID.
			return "", "", false
		}
		if err := ValidatePluginID(id, false); err != nil {
			return "", "", false
		}
		return id, serviceID[:len(PluginServiceIDPrefix)+len(id)+1], true
	case HostServiceIDPrefix:
		return "", HostServiceIDPrefix, true
	default:
		return "", "", false
	}
}

// ResolveLookupRpcClient resolves a LookupRpcClient directive with a plugin or plugin host.
//
// Resolves service IDs like:
//   - plugin/{plugin-id}/{service id}
//   - plugin-host/{service id}
//
// Returns nil, nil if the service ID does not match any of the known prefixes.
func ResolveLookupRpcClient(ctx context.Context, dir bifrost_rpc.LookupRpcClient, h LookupRpcClientHandler) (directive.Resolver, error) {
	serviceID := dir.LookupRpcServiceID()

	pluginID, stripServiceIDPrefix, ok := matchPluginServiceID(serviceID)
	if !ok {
		return nil, nil
	}

	return NewLookupRpcClientResolver(
		h,
		pluginID,
		stripServiceIDPrefix,
	), nil
}

// GetRpcClientCtr returns the rpc client container.
// The RPC client will be set when it becomes ready.
func (r *LookupRpcClientResolver) GetRpcClientCtr() *ccontainer.CContainer[*srpc.Client] {
	return r.rpcClientCtr
}

// Resolve publishes the current plugin client until the directive ends. A
// released client is cleared and relinquished before requesting its replacement.
func (r *LookupRpcClientResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	for {
		retry, err := r.resolveClient(ctx, handler)
		if err != nil || !retry {
			return err
		}
	}
}

// resolveClient retains one client while its value is published. It returns true
// when invalidation requires a replacement, false when the handler declines the
// lookup, and an error when lookup or the parent context fails.
func (r *LookupRpcClientResolver) resolveClient(ctx context.Context, handler directive.ResolverHandler) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	// Client invalidation ends this publication without canceling the directive.
	clientCtx, cancelClient := context.WithCancel(ctx)
	defer cancelClient()
	var client srpc.Client
	var release func()
	var err error
	if r.pluginID == "" {
		client, release, err = r.h.WaitPluginHostClient(ctx, cancelClient)
	} else {
		client, release, err = r.h.WaitPluginClient(ctx, cancelClient, r.pluginID)
	}
	if release != nil {
		defer release()
	}
	if err != nil || client == nil {
		return false, err
	}
	if clientCtx.Err() != nil {
		return true, ctx.Err()
	}

	// Expose the service-prefixed client only while its retained value is valid.
	if r.stripServiceIDPrefix != "" {
		client = srpc.NewPrefixClient(client, []string{r.stripServiceIDPrefix})
	}
	r.rpcClientCtr.SetValue(&client)
	defer r.rpcClientCtr.SetValue(nil)
	_, _ = handler.AddValue(client)
	defer handler.ClearValues()
	handler.MarkIdle(true)
	<-clientCtx.Done()
	return true, ctx.Err()
}

// LookupRpcClientResolver implements the directive resolver contract.
var _ directive.Resolver = (*LookupRpcClientResolver)(nil)
