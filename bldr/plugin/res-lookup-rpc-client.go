package bldr_plugin

import (
	"context"
	"strings"
	"sync"

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
	// Otherwise returns client, releaseFunc, nil
	WaitPluginHostClient(ctx context.Context, released func()) (srpc.Client, func(), error)

	// WaitPluginClient waits for an RPC client for a plugin.
	//
	// Released is a function to call if the client becomes invalid.
	// Returns nil, nil, err if any error.
	// Returns nil, nil, nil to skip resolving the client.
	// Otherwise returns client, releaseFunc, nil
	WaitPluginClient(ctx context.Context, released func(), pluginID string) (srpc.Client, func(), error)
}

// LookupRpcClientResolver resolves LookupRpcClient with the plugin or plugin host.
//
// Resolves service IDs like:
//   - plugin/{plugin-id}/{service id}
//   - plugin-host/{service id}
type LookupRpcClientResolver struct {
	// h is the handler
	h LookupRpcClientHandler
	// pluginID is the plugin identifier
	// if empty we are looking up the plugin host
	pluginID string
	// rpcClientCtr is the rpc client container
	rpcClientCtr *ccontainer.CContainer[*srpc.Client]
	// stripServiceIDPrefix is the prefix to strip from the service id, if any
	stripServiceIDPrefix string
}

// NewLookupRpcClientResolver constructs a new LookupRpcClientResolver.
//
// Usually you will want to use ResolveLookupRpcClient instead.
// If pluginID is empty, addresses the plugin host.
// stripServiceIDPrefix is the prefix to strip from the service id, if any
func NewLookupRpcClientResolver(h LookupRpcClientHandler, pluginID, stripServiceIDPrefix string) *LookupRpcClientResolver {
	return &LookupRpcClientResolver{
		h:                    h,
		pluginID:             pluginID,
		stripServiceIDPrefix: stripServiceIDPrefix,

		rpcClientCtr: ccontainer.NewCContainer[*srpc.Client](nil),
	}
}

// ResolveLookupRpcClient resolves a LookupRpcClient directive with a plugin or plugin host.
//
// Resolves service IDs like:
//   - plugin/{plugin-id}/{service id}
//   - plugin-host/{service id}
//
// Returns nil, nil if the service ID does not match any of the known prefixes.
// Returns an error if the plugin id is invalid.
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
		id, remoteServiceID, cutOk := strings.Cut(matchedService, "/")
		if !cutOk || remoteServiceID == "" || id == "" {
			// require the format: plugin/{plugin-id}/{service-id}
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

// Resolve resolves the values, emitting them to the handler.
func (r *LookupRpcClientResolver) Resolve(rctx context.Context, handler directive.ResolverHandler) error {
	ctx, ctxCancel := context.WithCancel(rctx)
	defer ctxCancel()

	for {
		_ = handler.ClearValues()
		r.rpcClientCtr.SetValue(nil)

		if ctx.Err() != nil {
			return context.Canceled
		}

		pluginID := r.pluginID
		var client srpc.Client
		var rel func()
		var err error

		releasedWaitCh := make(chan struct{})
		var releasedOnce sync.Once
		releasedFn := func() {
			releasedOnce.Do(func() {
				close(releasedWaitCh)
			})
		}

		if pluginID == "" {
			client, rel, err = r.h.WaitPluginHostClient(ctx, releasedFn)
		} else {
			client, rel, err = r.h.WaitPluginClient(ctx, releasedFn, pluginID)
		}
		if err != nil || client == nil {
			if rel != nil {
				rel()
			}
			return err
		}

		if r.stripServiceIDPrefix != "" {
			client = srpc.NewPrefixClient(client, []string{r.stripServiceIDPrefix})
		}

		value := client
		r.rpcClientCtr.SetValue(&value)
		_, _ = handler.AddValue(value)
		handler.MarkIdle(true)

		select {
		case <-ctx.Done():
		case <-releasedWaitCh:
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = (*LookupRpcClientResolver)(nil)
