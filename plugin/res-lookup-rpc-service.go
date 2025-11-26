package bldr_plugin

import (
	"context"
	"io"
	"strings"
	"sync"
	"sync/atomic"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// LookupRpcServiceResolver resolves LookupRpcService with the plugin or plugin host.
//
// Resolves service IDs like:
//   - plugin/{plugin-id}/{service id}
//   - plugin-host/{service id}
type LookupRpcServiceResolver struct {
	// h is the handler
	h LookupRpcClientHandler
	// pluginID is the plugin identifier
	// if empty we are looking up the plugin host
	pluginID string
	// stripServiceIDPrefix is the prefix to strip from the service id, if any
	stripServiceIDPrefix string
}

// NewLookupRpcServiceResolver constructs a new LookupRpcServiceResolver.
//
// Usually you will want to use ResolveLookupRpcService instead.
// If pluginID is empty, addresses the plugin host.
// stripServiceIDPrefix is the prefix to strip from the service id, if any
func NewLookupRpcServiceResolver(h LookupRpcClientHandler, pluginID, stripServiceIDPrefix string) *LookupRpcServiceResolver {
	return &LookupRpcServiceResolver{
		h:                    h,
		pluginID:             pluginID,
		stripServiceIDPrefix: stripServiceIDPrefix,
	}
}

// ResolveLookupRpcService resolves a LookupRpcService directive with a plugin or plugin host.
//
// Resolves service IDs like:
//   - plugin/{plugin-id}/{service id}
//   - plugin-host/{service id}
//
// Returns nil, nil if the service ID does not match any of the known prefixes.
// Returns an error if the plugin id is invalid.
func ResolveLookupRpcService(ctx context.Context, dir bifrost_rpc.LookupRpcService, h LookupRpcClientHandler) (directive.Resolver, error) {
	serviceID := dir.LookupRpcServiceID()

	// check if the service ID matches one of the known prefixes.
	matchedService, matchedPrefix := srpc.CheckStripPrefix(serviceID, []string{
		PluginServiceIDPrefix,
		HostServiceIDPrefix,
	})

	var pluginID, stripServiceIDPrefix string
	switch matchedPrefix {
	case PluginServiceIDPrefix:
		var remoteServiceID string
		var ok bool
		pluginID, remoteServiceID, ok = strings.Cut(matchedService, "/")
		if !ok || remoteServiceID == "" || pluginID == "" {
			// ignore: we require the following format:
			// plugin/{plugin-id}/{service-id}
			return nil, nil
		}
		if err := ValidatePluginID(pluginID, false); err != nil {
			// ignore it: invalid plugin id
			return nil, err
		}
		stripServiceIDPrefix = serviceID[:len(PluginServiceIDPrefix)+len(pluginID)+1]
	case HostServiceIDPrefix:
		stripServiceIDPrefix = HostServiceIDPrefix
	default:
		// no match
		return nil, nil
	}

	return NewLookupRpcServiceResolver(
		h,
		pluginID,
		stripServiceIDPrefix,
	), nil
}

// clientForwardingInvoker implements an Invoker that forwards calls to a Client.
type clientForwardingInvoker struct {
	client      srpc.Client
	stripPrefix string
}

// newClientForwardingInvoker creates a new forwarding invoker.
func newClientForwardingInvoker(client srpc.Client, stripPrefix string) srpc.Invoker {
	return &clientForwardingInvoker{
		client:      client,
		stripPrefix: stripPrefix,
	}
}

// InvokeMethod invokes the method by forwarding to the client.
func (f *clientForwardingInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	// Strip the prefix from serviceID if needed
	targetServiceID := serviceID
	if f.stripPrefix != "" && strings.HasPrefix(serviceID, f.stripPrefix) {
		targetServiceID = strings.TrimPrefix(serviceID, f.stripPrefix)
	}

	// If target service ID is empty after stripping, we can't forward
	if targetServiceID == "" {
		return false, nil
	}

	// Create outgoing stream via client
	outgoingStream, err := f.client.NewStream(strm.Context(), targetServiceID, methodID, nil)
	if err != nil {
		return true, errors.Wrap(err, "failed to create outgoing stream")
	}
	defer strm.Close()

	// Bridge the streams bidirectionally
	// Start a routine to write messages from server to client
	writeServerToClient := func() error {
		serverMsg := &srpc.RawMessage{}
		for {
			if err := outgoingStream.MsgRecv(serverMsg); err != nil {
				return err
			}
			if err := strm.MsgSend(serverMsg); err != nil {
				return err
			}
			serverMsg.Clear()
		}
	}

	var sendErr atomic.Pointer[error]
	go func() {
		defer outgoingStream.Close()
		if err := writeServerToClient(); err != nil && err != io.EOF {
			sendErr.Store(&err)
		}
	}()

	// Write messages from client to server
	writeClientToServer := func() error {
		clientMsg := &srpc.RawMessage{}
		for {
			if err := strm.MsgRecv(clientMsg); err != nil {
				return err
			}
			if err := outgoingStream.MsgSend(clientMsg); err != nil {
				return err
			}
			clientMsg.Clear()
		}
	}

	writeErr := writeClientToServer()
	if writeErr == nil || writeErr == io.EOF {
		readErr := sendErr.Load()
		if readErr != nil {
			writeErr = *readErr
		}
	}
	return true, writeErr
}

// Resolve resolves the values, emitting them to the handler.
func (r *LookupRpcServiceResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	handler.ClearValues()

	var client srpc.Client
	var rel func()
	var err error

	pluginID := r.pluginID
	releasedCh := make(chan struct{})
	releasedFn := sync.OnceFunc(func() {
		close(releasedCh)
	})

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
	defer rel()

	// Create an invoker that forwards calls to the client and strips the service id prefix
	invoker := newClientForwardingInvoker(client, r.stripServiceIDPrefix)
	value := invoker
	valueID, valueOk := handler.AddValue(value)
	handler.MarkIdle(true)

	select {
	case <-ctx.Done():
		return context.Canceled
	case <-releasedCh:
		if valueOk {
			_, _ = handler.RemoveValue(valueID)
		}
		// client became invalid, return to retry
		return nil
	}
}

// _ is a type assertion
var (
	_ directive.Resolver = ((*LookupRpcServiceResolver)(nil))
	_ srpc.Invoker       = ((*clientForwardingInvoker)(nil))
)
