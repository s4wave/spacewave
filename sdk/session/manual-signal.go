//go:build !tinygo && !goscript

package s4wave_session

import (
	"context"
	"io"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
	webrtc "github.com/pion/webrtc/v4"
	"github.com/pkg/errors"
	"github.com/quic-go/quic-go"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/peer"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	"github.com/s4wave/spacewave/net/util/rwc"
	"github.com/sirupsen/logrus"
)

// manualSignalDataChannelID is the datachannel label for QUIC-over-WebRTC.
const manualSignalDataChannelID = "bifrost-quic"

// ManualSignalTransport manages a WebRTC peer connection for manual SDP
// exchange. Unlike the bifrost WebRTC transport which uses trickle ICE via a
// signaling channel, this gathers all ICE candidates before producing the
// SDP, suitable for QR code or paste-based exchange.
type ManualSignalTransport struct {
	pc        *webrtc.PeerConnection
	dc        *webrtc.DataChannel
	identity  *p2ptls.Identity
	localPeer peer.ID
	offerer   bool
	le        *logrus.Entry

	gatherDone <-chan struct{}
	state      manualSignalTransportState
	closeOnce  sync.Once
	closeErr   error
}

var errManualSignalDataChannelClosed = errors.New("datachannel closed before link")
var errManualSignalDataChannelLinked = errors.New("datachannel already linked")

type manualSignalTransportState struct {
	bcast    broadcast.Broadcast
	dcRwc    io.ReadWriteCloser
	err      error
	closed   bool
	consumed bool
}

func (s *manualSignalTransportState) setReady(dcRwc io.ReadWriteCloser) bool {
	if dcRwc == nil {
		s.fail(errManualSignalDataChannelClosed)
		return false
	}

	var ready bool
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.closed || s.err != nil || s.dcRwc != nil || s.consumed {
			return
		}
		s.dcRwc = dcRwc
		ready = true
		bcast()
	})
	return ready
}

func (s *manualSignalTransportState) fail(err error) {
	if err == nil {
		return
	}
	var closeRwc io.ReadWriteCloser
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.err != nil {
			return
		}
		s.err = err
		closeRwc = s.dcRwc
		s.dcRwc = nil
		bcast()
	})
	if closeRwc != nil {
		_ = closeRwc.Close()
	}
}

func (s *manualSignalTransportState) close() bool {
	var closed bool
	var closeRwc io.ReadWriteCloser
	s.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		if s.closed {
			return
		}
		s.closed = true
		closed = true
		closeRwc = s.dcRwc
		s.dcRwc = nil
		bcast()
	})
	if closeRwc != nil {
		_ = closeRwc.Close()
	}
	return closed
}

func (s *manualSignalTransportState) waitReady(ctx context.Context) (io.ReadWriteCloser, error) {
	for {
		var dcRwc io.ReadWriteCloser
		var err error
		var closed bool
		var waitCh <-chan struct{}
		s.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			dcRwc = s.dcRwc
			err = s.err
			closed = s.closed
			if dcRwc != nil && err == nil && !closed {
				s.dcRwc = nil
				s.consumed = true
				return
			}
			if err == nil && !closed && !s.consumed {
				waitCh = getWaitCh()
			}
		})
		if err != nil {
			return nil, err
		}
		if closed {
			return nil, errManualSignalDataChannelClosed
		}
		if dcRwc != nil {
			return dcRwc, nil
		}
		if waitCh == nil {
			return nil, errManualSignalDataChannelLinked
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
	}
}

// NewManualSignalTransport creates a new transport with the given identity and
// ICE server configuration. The datachannel is pre-created in negotiated mode.
func NewManualSignalTransport(
	le *logrus.Entry,
	identity *p2ptls.Identity,
	localPeerID peer.ID,
	iceServers []webrtc.ICEServer,
) (*ManualSignalTransport, error) {
	se := webrtc.SettingEngine{}
	se.DetachDataChannels()
	api := webrtc.NewAPI(webrtc.WithSettingEngine(se))

	pc, err := api.NewPeerConnection(webrtc.Configuration{
		ICEServers: iceServers,
	})
	if err != nil {
		return nil, errors.Wrap(err, "create peer connection")
	}

	negotiated := true
	protocol := manualSignalDataChannelID
	ordered := false
	var channelID uint16 = 1
	dc, err := pc.CreateDataChannel(manualSignalDataChannelID, &webrtc.DataChannelInit{
		Negotiated: &negotiated,
		Protocol:   &protocol,
		ID:         &channelID,
		Ordered:    &ordered,
	})
	if err != nil {
		_ = pc.Close()
		return nil, errors.Wrap(err, "create data channel")
	}

	m := &ManualSignalTransport{
		pc:        pc,
		dc:        dc,
		identity:  identity,
		localPeer: localPeerID,
		le:        le,
	}

	dc.OnOpen(m.onDataChannelOpen)
	dc.OnClose(m.onDataChannelClose)
	dc.OnError(m.onDataChannelError)
	m.gatherDone = webrtc.GatheringCompletePromise(pc)

	return m, nil
}

// onDataChannelOpen detaches the datachannel for raw read/write access.
func (m *ManualSignalTransport) onDataChannelOpen() {
	dcRwc, err := m.dc.Detach()
	if err != nil {
		m.le.WithError(err).Warn("datachannel detach failed")
		m.state.fail(errors.Wrap(err, "detach datachannel"))
		return
	}
	m.onDataChannelReady(dcRwc)
}

func (m *ManualSignalTransport) onDataChannelReady(dcRwc io.ReadWriteCloser) {
	if !m.state.setReady(dcRwc) {
		if dcRwc != nil {
			_ = dcRwc.Close()
		}
	}
}

func (m *ManualSignalTransport) onDataChannelClose() {
	m.state.close()
}

func (m *ManualSignalTransport) onDataChannelError(err error) {
	if err == nil {
		err = errors.New("datachannel error")
	}
	m.state.fail(errors.Wrap(err, "datachannel error"))
}

// CreateOffer generates a complete SDP offer with all ICE candidates gathered.
// The caller is marked as the offerer for subsequent QUIC role selection.
func (m *ManualSignalTransport) CreateOffer(ctx context.Context) (string, error) {
	m.offerer = true

	offer, err := m.pc.CreateOffer(nil)
	if err != nil {
		return "", errors.Wrap(err, "create offer")
	}
	if err := m.pc.SetLocalDescription(offer); err != nil {
		return "", errors.Wrap(err, "set local description")
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-m.gatherDone:
	}

	desc := m.pc.LocalDescription()
	if desc == nil {
		return "", errors.New("local description is nil after gathering")
	}
	return desc.SDP, nil
}

// AcceptOffer accepts a remote SDP offer and returns a complete SDP answer
// with all ICE candidates gathered. The caller is marked as the answerer.
func (m *ManualSignalTransport) AcceptOffer(ctx context.Context, offerSDP string) (string, error) {
	m.offerer = false

	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  offerSDP,
	}
	if err := m.pc.SetRemoteDescription(offer); err != nil {
		return "", errors.Wrap(err, "set remote description")
	}

	answer, err := m.pc.CreateAnswer(nil)
	if err != nil {
		return "", errors.Wrap(err, "create answer")
	}
	if err := m.pc.SetLocalDescription(answer); err != nil {
		return "", errors.Wrap(err, "set local description")
	}

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-m.gatherDone:
	}

	desc := m.pc.LocalDescription()
	if desc == nil {
		return "", errors.New("local description is nil after gathering")
	}
	return desc.SDP, nil
}

// AcceptAnswer sets the remote SDP answer to complete the WebRTC connection.
func (m *ManualSignalTransport) AcceptAnswer(answerSDP string) error {
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  answerSDP,
	}
	return m.pc.SetRemoteDescription(answer)
}

// WaitLink waits for the datachannel to open and establishes a bifrost QUIC
// link over the WebRTC datachannel. The offerer listens for QUIC and the
// answerer dials, matching the bifrost convention.
func (m *ManualSignalTransport) WaitLink(
	ctx context.Context,
	linkCtx context.Context,
	remotePeerID peer.ID,
) (*transport_quic.Link, error) {
	dcRwc, err := m.state.waitReady(ctx)
	if err != nil {
		return nil, err
	}

	localAddr := peer.NewNetAddr(m.localPeer)
	remoteAddr := peer.NewNetAddr(remotePeerID)
	pconn := rwc.NewRwcPacketConn(dcRwc, localAddr, remoteAddr)

	linkOpts := &transport_quic.Opts{
		DisableDatagrams:        true,
		DisableKeepAlive:        true,
		DisablePathMtuDiscovery: true,
		MaxIdleTimeoutDur:       "60s",
	}

	var sess *quic.Conn
	if m.offerer {
		sess, err = transport_quic.ListenSession(ctx, m.le, linkOpts, pconn, m.identity, remotePeerID)
	} else {
		sess, _, err = transport_quic.DialSession(ctx, m.le, linkOpts, pconn, m.identity, remoteAddr, remotePeerID)
	}
	if err != nil {
		_ = pconn.Close()
		return nil, errors.Wrap(err, "quic session")
	}

	lnk, err := transport_quic.NewLink(
		linkCtx,
		m.le,
		&transport_quic.Opts{},
		0, // no registered transport UUID
		m.localPeer,
		localAddr,
		sess,
		func() { _ = m.Close() },
	)
	if err != nil {
		_ = sess.CloseWithError(0, "")
		_ = pconn.Close()
		return nil, errors.Wrap(err, "create link")
	}
	return lnk, nil
}

// IsOfferer returns true if this transport created the WebRTC offer (QUIC server).
func (m *ManualSignalTransport) IsOfferer() bool {
	return m.offerer
}

// Close closes the peer connection and releases resources.
func (m *ManualSignalTransport) Close() error {
	m.state.close()
	m.closeOnce.Do(func() {
		m.closeErr = m.pc.Close()
	})
	return m.closeErr
}
