package objecttype

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/net/peer"
)

// sessionPeerIDKey is the context key for the session peer ID.
type sessionPeerIDKey struct{}

// WithSessionPeerID returns a context with the session peer ID attached.
// The factory can use SessionPeerIDFromContext to retrieve it.
func WithSessionPeerID(ctx context.Context, peerID peer.ID) context.Context {
	return context.WithValue(ctx, sessionPeerIDKey{}, peerID)
}

// SessionPeerIDFromContext returns the session peer ID from the context.
// Returns empty peer.ID if not set.
func SessionPeerIDFromContext(ctx context.Context) peer.ID {
	v, _ := ctx.Value(sessionPeerIDKey{}).(peer.ID)
	return v
}

// engineIDKey is the context key for the world engine ID.
type engineIDKey struct{}

// WithEngineID returns a context with the world engine ID attached.
func WithEngineID(ctx context.Context, engineID string) context.Context {
	return context.WithValue(ctx, engineIDKey{}, engineID)
}

// EngineIDFromContext returns the world engine ID from the context.
// Returns empty string if not set.
func EngineIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(engineIDKey{}).(string)
	return v
}

// pluginBusKey is the context key for the plugin bus.
type pluginBusKey struct{}

// WithPluginBus returns a context with the plugin bus attached.
//
// The plugin bus receives plugin loads and serves the RPC services that loaded
// plugins call. It differs from the factory's bus when the Space runs inside a
// plugin, where the Space's own bus has no plugin hosts.
func WithPluginBus(ctx context.Context, b bus.Bus) context.Context {
	return context.WithValue(ctx, pluginBusKey{}, b)
}

// PluginBusFromContext returns the plugin bus from the context.
// Returns fallback if not set.
func PluginBusFromContext(ctx context.Context, fallback bus.Bus) bus.Bus {
	if v, _ := ctx.Value(pluginBusKey{}).(bus.Bus); v != nil {
		return v
	}
	return fallback
}
