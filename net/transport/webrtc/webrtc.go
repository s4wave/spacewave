package webrtc

import (
	"bytes"
	"context"
	"errors"
	"net"
	"slices"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	pion_transport "github.com/pion/transport/v4"
	"github.com/pion/webrtc/v4"
	"github.com/s4wave/spacewave/net/crypto"
	p2ptls "github.com/s4wave/spacewave/net/crypto/tls"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	"github.com/sirupsen/logrus"
)

// Option configures optional behavior of the WebRTC transport.
//
// Options are an injection seam used by tests to drive pion/ice over a virtual
// network with deterministic ICE timeouts. Production callers pass no options
// and get the browser/native defaults.
type Option func(*options)

// options holds the resolved optional configuration.
type options struct {
	// iceNet replaces the pion/ice network stack, if set.
	iceNet pion_transport.Net
	// iceDisconnectedTimeout overrides the ICE disconnected timeout, if non-zero.
	iceDisconnectedTimeout time.Duration
	// iceFailedTimeout overrides the ICE failed timeout, if non-zero.
	iceFailedTimeout time.Duration
	// iceKeepaliveInterval overrides the ICE keepalive interval, if non-zero.
	iceKeepaliveInterval time.Duration
}

// WithICENet sets the pion/ice network stack, replacing the default OS stack.
func WithICENet(net pion_transport.Net) Option {
	return func(o *options) { o.iceNet = net }
}

// WithICETimeouts overrides the pion/ice consent timeouts.
//
// A zero value leaves the corresponding pion default in place.
func WithICETimeouts(disconnected, failed, keepalive time.Duration) Option {
	return func(o *options) {
		o.iceDisconnectedTimeout = disconnected
		o.iceFailedTimeout = failed
		o.iceKeepaliveInterval = keepalive
	}
}

// TransportType is the transport type identifier for this transport.
const TransportType = "webrtc"

// ControllerID is the WebRTC controller ID.
const ControllerID = "bifrost/webrtc"

// SignalingProtocolID is the default protocol id to use for the signaling client.
var SignalingProtocolID protocol.ID = signaling.ProtocolID

// Version is the version of the implementation.
var Version = controller.MustParseVersion("0.0.1")

// WebRTC implements a WebRTC transport.
type WebRTC struct {
	// ctx is the context
	ctx context.Context
	// le is the logger
	le *logrus.Entry
	// b is the bus
	b bus.Bus
	// conf is the webrtc-signal-rpc config
	conf *Config
	// tptType is the transport type
	tptType string
	// peerID is the local peer id
	peerID peer.ID
	// privKey is the local private key
	privKey crypto.PrivKey
	// uuid is the unique id
	uuid uint64
	// laddr is the local address
	laddr net.Addr
	// handler is the transport handler
	handler transport.TransportHandler
	// opts are extra options
	opts *transport_quic.Opts
	// identity is the p2ptls identity
	identity *p2ptls.Identity
	// webrtcConf is the webrtc configuration
	webrtcConf *webrtc.Configuration
	// webrtcApi is the webrtc api
	webrtcApi *webrtc.API
	// sessionTrackers contains a mapping from peer id to ongoing session
	// peer id is encoded as string
	sessionTrackers *keyed.KeyedRefCount[string, *sessionTracker]
	// bcast guards below fields
	bcast broadcast.Broadcast
	// relSignalHandler releases the signal handler controller
	relSignalHandler func()
	// incomingSessions contains the per-peer signal-ingress leases.
	incomingSessions map[string]*signalIngress
}

// NewWebRTC builds a new WebRTC transport.
//
// ServeHTTP is implemented and can be used with a standard HTTP mux.
// Optionally listens on an address.
func NewWebRTC(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	conf *Config,
	pKey crypto.PrivKey,
	c transport.TransportHandler,
	opts ...Option,
) (*WebRTC, error) {
	// Apply non-nil options to the transport configuration.
	var o options
	for _, opt := range opts {
		if opt != nil {
			opt(&o)
		}
	}

	// Resolve the configured transport type.
	tptType := conf.GetTransportType()
	if tptType == "" {
		tptType = TransportType
	}

	// Derive the local peer identity.
	peerID, err := peer.IDFromPrivateKey(pKey)
	if err != nil {
		return nil, err
	}

	// Build the TLS identity used by signaling sessions.
	identity, err := p2ptls.NewIdentity(pKey)
	if err != nil {
		return nil, err
	}

	// Clone or initialize the QUIC options.
	quicOpts := conf.GetQuic()
	if quicOpts == nil {
		quicOpts = &transport_quic.Opts{}
	} else {
		quicOpts = quicOpts.CloneVT()
	}

	// Configure QUIC for WebRTC data channels.
	quicOpts.DisableDatagrams = true
	quicOpts.DisableKeepAlive = false
	quicOpts.DisablePathMtuDiscovery = true

	// Build the WebRTC API and configuration.
	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	applyICEOptions(&settingEngine, &o)
	webrtcApi := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	webrtcConf := conf.WebRtc.ToWebRtcConfiguration()

	// Assemble the transport state.
	tpt := &WebRTC{
		ctx:        ctx,
		b:          b,
		le:         le,
		conf:       conf,
		tptType:    tptType,
		peerID:     peerID,
		privKey:    pKey,
		uuid:       NewTransportUUID("webrtc-signal-rpc", peerID),
		laddr:      peer.NewNetAddr(peerID),
		handler:    c,
		opts:       quicOpts,
		webrtcApi:  webrtcApi,
		webrtcConf: webrtcConf,
		identity:   identity,
	}

	// Initialize ingress leases and keyed session trackers.
	tpt.incomingSessions = make(map[string]*signalIngress)
	tpt.sessionTrackers = keyed.NewKeyedRefCount[string, *sessionTracker](
		tpt.newSessionTracker,
		keyed.WithExitLogger[string, *sessionTracker](le),
		keyed.WithBackoff[string, *sessionTracker](func(_ string) cbackoff.BackOff {
			return conf.GetBackoff().Construct()
		}),
	)

	return tpt, nil
}

// GetUUID returns a host-unique ID for this transport.
func (w *WebRTC) GetUUID() uint64 {
	return w.uuid
}

// GetPeerID returns the peer ID.
func (w *WebRTC) GetPeerID() peer.ID {
	return w.peerID
}

// GetVerbose gets if verbose logging is enabled.
func (w *WebRTC) GetVerbose() bool {
	return w.conf.GetVerbose()
}

// MatchTransportType checks if the given transport type ID matches this transport.
// If returns true, the transport controller will call DialPeer with that tptaddr.
// E.x.: "udp-quic" or "ws"
func (w *WebRTC) MatchTransportType(transportType string) bool {
	return transportType == w.tptType
}

// GetPeerDialer returns the dialing information for a peer.
// Called when resolving EstablishLink.
// Return nil, nil to indicate not found or unavailable.
func (w *WebRTC) GetPeerDialer(ctx context.Context, peerID peer.ID) (*dialer.DialerOpts, error) {
	peerIDStr := peerID.String()
	if slices.Contains(w.conf.GetBlockPeers(), peerIDStr) {
		return nil, nil
	}

	if peerDialer, ok := w.conf.GetDialers()[peerIDStr]; ok {
		return peerDialer, nil
	}
	if w.conf.GetAllPeers() {
		// Session transports enable deterministic offers because both peers see
		// the same EstablishLinkWithPeer request.
		if w.conf.GetAllPeersLowerPeerOffers() && bytes.Compare([]byte(w.peerID), []byte(peerID)) >= 0 {
			return nil, nil
		}
		return &dialer.DialerOpts{
			Address: "webrtc",
			Backoff: w.conf.GetBackoff(),
		}, nil
	}

	return nil, nil
}

// Execute executes the transport as configured, returning any fatal error.
func (w *WebRTC) Execute(ctx context.Context) error {
	// Start session trackers for the execution context.
	w.sessionTrackers.SetContext(ctx, true)

	// Register the signaling handler when listening is enabled.
	if !w.conf.GetDisableListen() {
		handler := NewWebRTCSignalHandler(w)
		relSignalHandler, err := w.b.AddController(ctx, handler, nil)
		if err != nil {
			return err
		}
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if w.relSignalHandler != nil {
				w.relSignalHandler()
			}
			w.relSignalHandler = relSignalHandler
		})
	}

	return nil
}

// DialPeer dials a peer given an address. The yielded link should be
// emitted to the transport handler. DialPeer should return nil if the link
// was established. DialPeer will then not be called again for the same peer
// ID and address tuple until the yielded link is lost.
// Returns fatal and error.
func (w *WebRTC) DialPeer(
	ctx context.Context,
	peerID peer.ID,
	addr string,
) (olnk link.Link, fatal bool, err error) {
	// Resolve the peer identity and reject blocked peers.
	peerIDStr := peerID.String()
	if slices.Contains(w.conf.GetBlockPeers(), peerIDStr) {
		return nil, false, nil
	}

	var ref *keyed.KeyedRef[string, *sessionTracker]
	var waitCh <-chan struct{}
	var tkr *sessionTracker
	var lnk *transport_quic.Link

	w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Add or reuse the keyed session tracker reference.
		var existed bool
		ref, tkr, existed, err = w.addSessionTrackerRef(peerIDStr)

		// Notify signal handlers if it didn't exist
		if err == nil && !existed {
			broadcast()
		}
		if tkr != nil {
			lnk = tkr.link
		}
		waitCh = getWaitCh()
	})
	if ref != nil {
		defer ref.Release()
	}
	if err != nil {
		return nil, false, err
	}

	// Wait until the session tracker publishes a link.
	for lnk == nil {
		select {
		case <-ctx.Done():
			return nil, false, context.Canceled
		case <-waitCh:
		}

		// Refresh the link snapshot and wait channel under the broadcast lock.
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			lnk = tkr.link
			waitCh = getWaitCh()
		})
	}

	return lnk, false, nil
}

// Close closes the transport, returning any errors closing.
func (w *WebRTC) Close() error {
	// Stop session trackers and release the signaling handler.
	w.sessionTrackers.ClearContext()
	w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if w.relSignalHandler != nil {
			w.relSignalHandler()
			w.relSignalHandler = nil
		}
	})
	return nil
}

// addSessionTrackerRef validates the peer id string and adds a session tracker ref.
func (w *WebRTC) addSessionTrackerRef(peerIDStr string) (*keyed.KeyedRef[string, *sessionTracker], *sessionTracker, bool, error) {
	// Parse the remote identity and public key before tracking its session.
	peerID, peerPub, err := peer.ParsePeerIDWithPubKey(peerIDStr)
	if err != nil {
		return nil, nil, false, err
	}

	// Reject attempts to establish a session with the local peer.
	if w.peerID.MatchesPublicKey(peerPub) {
		return nil, nil, false, errors.New("signaling: cannot self-dial")
	}

	ref, tkr, existed := w.sessionTrackers.AddKeyRef(peerID.String())
	return ref, tkr, existed, nil
}

// _ is a type assertion.
var (
	_ transport.Transport    = (*WebRTC)(nil)
	_ dialer.TransportDialer = (*WebRTC)(nil)
)
