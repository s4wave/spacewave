package pubsub_controller

import (
	"context"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/pubsub"
)

// streamHandler handles HandleMountedStream directives
type streamHandler struct {
	// c is the controller
	c *Controller
	// ps is the pubsub
	ps pubsub.PubSub
}

// newStreamHandler builds a new stream handler
func newStreamHandler(
	c *Controller,
	ps pubsub.PubSub,
) *streamHandler {
	return &streamHandler{c: c, ps: ps}
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
func (s *streamHandler) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	s.c.le.WithField("protocol-id", ms.GetProtocolID()).
		Info("pubsub stream opened (by them)")
	s.ps.AddPeerStream(pubsub.NewPeerLinkTuple(ms.GetLink()), false, ms)
	return nil
}

// _ is a type assertion
var _ link.MountedStreamHandler = ((*streamHandler)(nil))
