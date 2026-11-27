package bldr_plugin

import (
	"context"
	"encoding/base64"

	"github.com/aperturerobotics/starpc/rpcstream"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
)

// ComponentID encodes the caller as the PluginRpc component ID the host opens
// on the target plugin. ParsePluginCallerComponentID decodes it.
func (c *PluginCaller) ComponentID() string {
	data, _ := c.MarshalVT()
	return base64.RawURLEncoding.EncodeToString(data)
}

// ParsePluginCallerComponentID decodes a PluginRpc component ID into the
// caller the host stamped on it. It refuses an ID that does not decode or
// identifies no valid plugin.
func ParsePluginCallerComponentID(componentID string) (*PluginCaller, error) {
	// Decode the message and require a valid caller plugin ID.
	data, err := base64.RawURLEncoding.DecodeString(componentID)
	if err != nil {
		return nil, errors.Wrap(err, "decode plugin caller")
	}
	caller := &PluginCaller{}
	if err := caller.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode plugin caller")
	}
	if err := ValidatePluginID(caller.GetPluginId(), false); err != nil {
		return nil, errors.Wrap(err, "plugin caller")
	}
	return caller, nil
}

// pluginCallerKey is the context key for the PluginCaller.
type pluginCallerKey struct{}

// WithPluginCaller returns ctx carrying caller.
func WithPluginCaller(ctx context.Context, caller *PluginCaller) context.Context {
	return context.WithValue(ctx, pluginCallerKey{}, caller)
}

// PluginCallerFromContext returns the plugin that made the call ctx belongs
// to, or nil for a call that did not arrive through PluginRpc.
func PluginCallerFromContext(ctx context.Context) *PluginCaller {
	caller, _ := ctx.Value(pluginCallerKey{}).(*PluginCaller)
	return caller
}

// ServePluginRpc serves a PluginRpc stream with the invoker for its caller.
// It refuses a stream whose component ID is not a host-stamped caller. Every
// call the invoker serves carries the caller on its stream context, and so
// does every stream nested beneath it, such as a Resource's calls.
func ServePluginRpc(strm rpcstream.RpcStream, invokerFor func(caller *PluginCaller) srpc.Invoker) error {
	return rpcstream.HandleRpcStream(strm, func(_ context.Context, componentID string, _ func()) (srpc.Invoker, func(), error) {
		caller, err := ParsePluginCallerComponentID(componentID)
		if err != nil {
			return nil, nil, err
		}
		return &pluginCallerInvoker{caller: caller, inv: invokerFor(caller)}, nil, nil
	})
}

// pluginCallerInvoker puts its caller on the context of each call it serves.
type pluginCallerInvoker struct {
	caller *PluginCaller
	inv    srpc.Invoker
}

// InvokeMethod serves the call with the caller on its stream context.
func (i *pluginCallerInvoker) InvokeMethod(serviceID, methodID string, strm srpc.Stream) (bool, error) {
	ctx := WithPluginCaller(strm.Context(), i.caller)
	return i.inv.InvokeMethod(serviceID, methodID, srpc.NewStreamWithContext(strm, ctx))
}

// _ is a type assertion
var _ srpc.Invoker = (*pluginCallerInvoker)(nil)
