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
	// rpc is the rpc to use
	rpc *queuedRPC
	// b is the bus
	b bus.Bus
}

// NewMountedStreamHandler constructs the mounted stream handler.
func NewMountedStreamHandler(
	le *logrus.Entry,
	bus bus.Bus,
	rpc *queuedRPC,
) (*MountedStreamHandler, error) {
	return &MountedStreamHandler{le: le, rpc: rpc, b: bus}, nil
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
func (m *MountedStreamHandler) HandleMountedStream(
	ctx context.Context,
	strm link.MountedStream,
) error {
	rpc := m.rpc.rpc
	s := strm.GetStream()
	_, estLinkInst, err := m.b.AddDirective(
		link.NewEstablishLinkWithPeer(strm.GetLink().GetLocalPeer(), strm.GetPeerID()),
		nil,
	)
	if err != nil {
		return err
	}

	go func() {
		defer estLinkInst.Release()

		if err := rpc.Send(&stream_api.Data{
			State: stream_api.StreamState_StreamState_ESTABLISHED,
		}); err != nil {
			s.Close()
			return
		}

		if err := stream_api.AttachRPCToStream(rpc, s, nil); err != nil &&
			err != io.EOF &&
			err != context.Canceled {
			m.le.WithError(err).Warn("rpc stream returned an error")
		}
	}()
	return nil
}

var _ link.MountedStreamHandler = ((*MountedStreamHandler)(nil))
