package stream_echo

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ioproxy"
	"github.com/s4wave/spacewave/net/link"
	"github.com/sirupsen/logrus"
)

// MountedStreamHandler implements the mounted stream handler.
type MountedStreamHandler struct {
	// le is the logger entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
}

// NewMountedStreamHandler constructs the mounted stream handler.
func NewMountedStreamHandler(le *logrus.Entry, bus bus.Bus) (*MountedStreamHandler, error) {
	return &MountedStreamHandler{le: le, bus: bus}, nil
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
func (m *MountedStreamHandler) HandleMountedStream(
	ctx context.Context,
	strm link.MountedStream,
) error {
	// Hold the source link while echoing the mounted stream.
	_, elRef, err := m.bus.AddDirective(
		link.NewEstablishLinkWithPeer(strm.GetLink().GetLocalPeer(), strm.GetPeerID()),
		nil,
	)
	if err != nil {
		return err
	}

	// Echo the stream asynchronously so the handler returns promptly.
	go func() {
		// Release the link reference when the echo routine exits.
		defer elRef.Release()

		// Prepare the echo stream and cancellation context.
		m.le.Debug("echoing stream")
		s := strm.GetStream()
		subCtx, subCtxCancel := context.WithCancel(ctx)
		defer subCtxCancel()

		// Proxy the stream back to itself until either side closes.
		ioproxy.ProxyStreams(s, s, subCtxCancel)

		// Wait for the proxy to finish before releasing the reference.
		<-subCtx.Done()
	}()
	return nil
}

// _ is a type assertion
var _ link.MountedStreamHandler = (*MountedStreamHandler)(nil)
