package bldr_plugin

import (
	"context"
	"io"
	"strings"
	"sync"

	bifrost_rpc "github.com/aperturerobotics/bifrost/rpc"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
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

	le := logrus.WithFields(logrus.Fields{
		"service": targetServiceID,
		"method":  methodID,
	})
	le.Debug("forwarding invoker: opening outgoing stream")

	// Create outgoing stream via client
	outgoingStream, err := f.client.NewStream(strm.Context(), targetServiceID, methodID, nil)
	if err != nil {
		le.WithError(err).Warn("forwarding invoker: outgoing stream open failed")
		return true, errors.Wrap(err, "failed to create outgoing stream")
	}
	le.Debug("forwarding invoker: outgoing stream opened, starting bridge")

	// NOTE: do not defer strm.Close() here. strm.Close() writes a CallCancel
	// packet which sets remoteErr=context.Canceled on the client, breaking
	// server-streaming RPCs. The caller (invokeRPC) handles stream completion
	// by sending CallData(complete=true) and closing the writer.

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

	serverDone := make(chan error, 1)
	go func() {
		defer outgoingStream.Close()
		err := writeServerToClient()
		le.WithError(err).Debug("forwarding invoker: server->client exited")
		serverDone <- err
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
	le.WithError(writeErr).Debug("forwarding invoker: client->server exited")

	// Client closed send side (EOF): forward the half-close to the
	// outgoing stream and wait for the server->client direction to finish.
	// This handles server-streaming RPCs where the client sends one message
	// then calls CloseSend() before reading responses.
	if writeErr == nil || writeErr == io.EOF {
		_ = outgoingStream.CloseSend()
		srvErr := <-serverDone
		if srvErr != nil && srvErr != io.EOF {
			writeErr = srvErr
		} else {
			writeErr = nil
		}
	}

	le.WithError(writeErr).Debug("forwarding invoker: bridge exited")
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
