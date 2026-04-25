package s4wave_session

import (
	"context"
	"io"
	"sync"

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
var manualSignalDataChannelID = "bifrost-quic"

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

	mu     sync.Mutex
	dcRwc  io.ReadWriteCloser
	dcOpen chan struct{}
	closed bool
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
		dcOpen:    make(chan struct{}),
	}

	dc.OnOpen(m.onDataChannelOpen)
	m.gatherDone = webrtc.GatheringCompletePromise(pc)

	return m, nil
}

// onDataChannelOpen detaches the datachannel for raw read/write access.
func (m *ManualSignalTransport) onDataChannelOpen() {
	dcRwc, err := m.dc.Detach()
	if err != nil {
		m.le.WithError(err).Warn("datachannel detach failed")
		return
	}
	m.mu.Lock()
	m.dcRwc = dcRwc
	close(m.dcOpen)
	m.mu.Unlock()
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
func (m *ManualSignalTransport) WaitLink(ctx context.Context, remotePeerID peer.ID) (*transport_quic.Link, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-m.dcOpen:
	}

	m.mu.Lock()
	dcRwc := m.dcRwc
	m.mu.Unlock()
	if dcRwc == nil {
		return nil, errors.New("datachannel closed before link")
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
	var err error
	if m.offerer {
		sess, err = transport_quic.ListenSession(ctx, m.le, linkOpts, pconn, m.identity, remotePeerID)
	} else {
		sess, _, err = transport_quic.DialSession(ctx, m.le, linkOpts, pconn, m.identity, remoteAddr, remotePeerID)
	}
	if err != nil {
		return nil, errors.Wrap(err, "quic session")
	}

	lnk, err := transport_quic.NewLink(
		ctx,
		m.le,
		&transport_quic.Opts{},
		0, // no registered transport UUID
		m.localPeer,
		localAddr,
		sess,
		func() { _ = m.Close() },
	)
	if err != nil {
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
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()
	return m.pc.Close()
}
