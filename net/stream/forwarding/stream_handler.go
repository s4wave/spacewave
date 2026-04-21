package stream_forwarding

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	ma "github.com/aperturerobotics/go-multiaddr"
	manet "github.com/aperturerobotics/go-multiaddr/net"
	"github.com/aperturerobotics/util/ioproxy"
	"github.com/s4wave/spacewave/net/link"
	"github.com/sirupsen/logrus"
)

// MountedStreamHandler implements the mounted stream handler.
type MountedStreamHandler struct {
	// le is the logger entry
	le *logrus.Entry
	// dialMa is the multiaddr to dial
	dialMa ma.Multiaddr
	// bus is the controller bus
	bus bus.Bus
}

// NewMountedStreamHandler constructs the mounted stream handler.
func NewMountedStreamHandler(le *logrus.Entry, bus bus.Bus, dialMa ma.Multiaddr) (*MountedStreamHandler, error) {
	le = le.WithField("dial-multiaddr", dialMa.String())
	return &MountedStreamHandler{le: le, dialMa: dialMa, bus: bus}, nil
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
func (m *MountedStreamHandler) HandleMountedStream(
	ctx context.Context,
	strm link.MountedStream,
) error {
	_, elRef, err := m.bus.AddDirective(
		link.NewEstablishLinkWithPeer(strm.GetLink().GetLocalPeer(), strm.GetPeerID()),
		nil,
	)
	if err != nil {
		return err
	}
	go func() {
		s := strm.GetStream()
		defer func() {
			elRef.Release()
			s.Close()
		}()

		// Attempt to dial the target.
		m.le.Debug("dialing to forward stream")
		conn, err := (&manet.Dialer{}).DialContext(ctx, m.dialMa)
		if err != nil {
			m.le.WithError(err).Warn("unable to dial to forward stream")
			return
		}

		m.le.Debug("connection opened")
		subCtx, subCtxCancel := context.WithCancel(ctx)
		defer subCtxCancel()
		ioproxy.ProxyStreams(conn, s, subCtxCancel)
		<-subCtx.Done()
		m.le.Debug("connection closing")

		// note: establishlink is released in defer above.
	}()
	return nil
}

var _ link.MountedStreamHandler = ((*MountedStreamHandler)(nil))
