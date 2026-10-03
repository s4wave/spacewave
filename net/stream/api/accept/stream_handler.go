package stream_api_accept

import (
	"context"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/net/link"
	stream_api "github.com/s4wave/spacewave/net/stream/api/rpc"
	"github.com/sirupsen/logrus"
)

// MountedStreamHandler implements the mounted stream handler.
type MountedStreamHandler struct {
	// le is the logger entry
	le *logrus.Entry
	// rpcCh supplies one waiting RPC per accepted stream.
	rpcCh <-chan *queuedRPC
	// b is the bus
	b bus.Bus
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
func (m *MountedStreamHandler) HandleMountedStream(
	ctx context.Context,
	strm link.MountedStream,
) error {
	// Keep the peer link established while its mounted stream serves an RPC.
	s := strm.GetStream()
	_, estLinkInst, err := m.b.AddDirective(
		link.NewEstablishLinkWithPeer(strm.GetLink().GetLocalPeer(), strm.GetPeerID()),
		nil,
	)
	if err != nil {
		return err
	}

	// Serve the next queued RPC asynchronously on the accepted stream.
	go func() {
		// Wait for a queued RPC while retaining stream and link cleanup.
		defer estLinkInst.Release()
		defer s.Close()
		var queued *queuedRPC
		select {
		case <-ctx.Done():
			return
		case queued = <-m.rpcCh:
		}
		rpc := queued.rpc

		// Notify the RPC client that its peer stream is established.
		if err := rpc.Send(&stream_api.Data{
			State: stream_api.StreamState_StreamState_ESTABLISHED,
		}); err != nil {
			queued.doneCb(err)
			return
		}

		// Transfer RPC data through the peer stream and report completion.
		err := stream_api.AttachRPCToStream(rpc, s, nil)
		queued.doneCb(err)
		if err != nil &&
			err != io.EOF &&
			err != context.Canceled {
			m.le.WithError(err).Warn("rpc stream returned an error")
		}
	}()
	return nil
}

// _ is a type assertion
var _ link.MountedStreamHandler = (*MountedStreamHandler)(nil)
