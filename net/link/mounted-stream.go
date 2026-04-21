package link

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
)

// MountedStream is a stream attached to a Link. This is produced and managed by
// the link controller. A mounted stream is produced after the initial stream
// negotiation is completed.
type MountedStream interface {
	// GetStream returns the underlying stream object.
	GetStream() stream.Stream
	// GetProtocolID returns the protocol ID of the stream.
	GetProtocolID() protocol.ID
	// GetOpenOpts returns the options used to open the stream.
	GetOpenOpts() stream.OpenOpts
	// GetPeerID returns the peer ID for the other end of the stream.
	GetPeerID() peer.ID
	// GetLink returns the associated link carrying the stream.
	GetLink() MountedLink
}

// MountedStreamHandler handles an incoming mounted stream.
type MountedStreamHandler interface {
	// HandleMountedStream handles an incoming mounted stream.
	//
	// This function should return as soon as possible, and start
	// additional goroutines to manage the lifecycle of the stream.
	//
	// The context will be canceled when the Link closes.
	// The context will /not/ be canceled when ms closes.
	// Any returned error indicates the stream should be closed.
	HandleMountedStream(ctx context.Context, ms MountedStream) error
}

// MountedStreamContext is the value attached to a Context containing
// information about the current mounted stream.
type MountedStreamContext = MountedStream

// mountedStreamContextKey is the context key used for WithValue.
type mountedStreamContextKey struct{}

// WithMountedStreamContext attaches a MountedStreamContext to a Context.
func WithMountedStreamContext(ctx context.Context, msc MountedStreamContext) context.Context {
	return context.WithValue(ctx, mountedStreamContextKey{}, msc)
}

// GetMountedStreamContext returns the MountedStreamContext from the Context or nil if unset.
func GetMountedStreamContext(ctx context.Context) MountedStreamContext {
	val := ctx.Value(mountedStreamContextKey{})
	msc, ok := val.(MountedStreamContext)
	if !ok || msc == nil {
		return nil
	}
	return msc
}

// ErrNoMountedStreamContext is returned if there was no MountedStreamContext.
var ErrNoMountedStreamContext = errors.New("no mounted stream context")

// MustGetMountedStreamContext returns the MountedStreamContext from the Context or an error if unset.
func MustGetMountedStreamContext(ctx context.Context) (MountedStreamContext, error) {
	msc := GetMountedStreamContext(ctx)
	if msc == nil {
		return nil, ErrNoMountedStreamContext
	}
	return msc, nil
}
