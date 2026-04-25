package objecttype

import (
	"context"

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
