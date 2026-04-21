package stream_relay

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ioproxy"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// MountedStreamHandler implements the mounted stream handler.
type MountedStreamHandler struct {
	// le is the logger entry
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// targetPeerID is the peer ID to relay to
	targetPeerID peer.ID
	// targetProtocolID is the protocol ID to relay to
	targetProtocolID protocol.ID
}

// NewMountedStreamHandler constructs the mounted stream handler.
func NewMountedStreamHandler(le *logrus.Entry, bus bus.Bus, targetPeerID peer.ID, targetProtocolID protocol.ID) (*MountedStreamHandler, error) {
	le = le.WithFields(logrus.Fields{
		"dst-peer":        targetPeerID.String(),
		"dst-protocol-id": targetProtocolID,
	})
	return &MountedStreamHandler{le: le, targetPeerID: targetPeerID, targetProtocolID: targetProtocolID, bus: bus}, nil
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
func (m *MountedStreamHandler) HandleMountedStream(
	ctx context.Context,
	strm link.MountedStream,
) error {
	localPeerID, remotePeerID := strm.GetLink().GetLocalPeer(), strm.GetPeerID()
	_, elRef, err := m.bus.AddDirective(
		link.NewEstablishLinkWithPeer(localPeerID, remotePeerID),
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

		// Emit directive to relay stream to target peer
		m.le.Debug("relaying stream to target peer")
		outMstrm, rel, err := link.OpenStreamWithPeerEx(ctx, m.bus, m.targetProtocolID, localPeerID, m.targetPeerID, 0, strm.GetOpenOpts())
		if err != nil {
			m.le.WithError(err).Warn("unable to relay stream to target peer")
			return
		}
		rel()

		outStrm := outMstrm.GetStream()
		defer outStrm.Close()

		m.le.Debug("connection opened")
		subCtx, subCtxCancel := context.WithCancel(ctx)
		defer subCtxCancel()
		ioproxy.ProxyStreams(s, outStrm, subCtxCancel)
		<-subCtx.Done()
		m.le.Debug("connection closing")

		// note: establishlink is released in defer above.
	}()
	return nil
}

// _ is a type assertion
var _ link.MountedStreamHandler = ((*MountedStreamHandler)(nil))
