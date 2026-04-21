package webrtc

import (
	"context"
	"errors"
	"net"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	cbackoff "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
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

// TransportType is the transport type identifier for this transport.
const TransportType = "webrtc"

// ControllerID is the WebRTC controller ID.
const ControllerID = "bifrost/webrtc"

// SignalingProtocolID is the default protocol id to use for the signaling client.
var SignalingProtocolID protocol.ID = signaling.ProtocolID

// Version is the version of the implementation.
var Version = semver.MustParse("0.0.1")

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
	// incomingSessions contains the set of sessions that were started due to
	// HandleSignalPeer directives. These references are dropped when a link
	// closes in order to prevent "bouncing" (repeatedly re-connecting).
	incomingSessions map[string]*keyed.KeyedRef[string, *sessionTracker]
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
) (*WebRTC, error) {
	tptType := conf.GetTransportType()
	if tptType == "" {
		tptType = TransportType
	}

	peerID, err := peer.IDFromPrivateKey(pKey)
	if err != nil {
		return nil, err
	}

	identity, err := p2ptls.NewIdentity(pKey)
	if err != nil {
		return nil, err
	}

	quicOpts := conf.GetQuic()
	if quicOpts == nil {
		quicOpts = &transport_quic.Opts{}
	} else {
		quicOpts = quicOpts.CloneVT()
	}

	// set webrtc-signal-rpc-specific quic opts
	quicOpts.DisableDatagrams = true
	quicOpts.DisableKeepAlive = false
	quicOpts.DisablePathMtuDiscovery = true

	// Setup the webrtc API
	settingEngine := webrtc.SettingEngine{}
	settingEngine.DetachDataChannels()
	webrtcApi := webrtc.NewAPI(webrtc.WithSettingEngine(settingEngine))
	webrtcConf := conf.WebRtc.ToWebRtcConfiguration()

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

	// The session tracker starts when we want a session with a remote peer.
	tpt.incomingSessions = make(map[string]*keyed.KeyedRef[string, *sessionTracker])
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

	if w.conf.GetAllPeers() {
		return &dialer.DialerOpts{
			Address: "webrtc",
			Backoff: w.conf.GetBackoff(),
		}, nil
	}

	return w.conf.GetDialers()[peerIDStr], nil
}

// Execute executes the transport as configured, returning any fatal error.
func (w *WebRTC) Execute(ctx context.Context) error {
	// Startup session trackers and signaling client
	w.sessionTrackers.SetContext(ctx, true)

	// If listening isn't disabled, handle incoming signals.
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
	// Ignore the address, since there is no address associated w/ WebRTC connections.
	// Get the peer ID string
	peerIDStr := peerID.String()
	if slices.Contains(w.conf.GetBlockPeers(), peerIDStr) {
		return nil, false, nil
	}

	var ref *keyed.KeyedRef[string, *sessionTracker]
	var waitCh <-chan struct{}
	var tkr *sessionTracker
	var lnk *transport_quic.Link

	w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		// Add the session reference.
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

	// Wait for the link to be established
	for lnk == nil {
		select {
		case <-ctx.Done():
			return nil, false, context.Canceled
		case <-waitCh:
		}

		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			lnk = tkr.link
			waitCh = getWaitCh()
		})
	}

	return lnk, false, nil
}

// Close closes the transport, returning any errors closing.
func (w *WebRTC) Close() error {
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
	// assert that we can extract the public key from the peer id
	peerID, peerPub, err := peer.ParsePeerIDWithPubKey(peerIDStr)
	if err != nil {
		return nil, nil, false, err
	}

	// assert that we are not trying to open a session with ourselves
	if w.peerID.MatchesPublicKey(peerPub) {
		return nil, nil, false, errors.New("signaling: cannot self-dial")
	}

	ref, tkr, existed := w.sessionTrackers.AddKeyRef(peerID.String())
	return ref, tkr, existed, nil
}

// _ is a type assertion.
var (
	_ transport.Transport    = ((*WebRTC)(nil))
	_ dialer.TransportDialer = ((*WebRTC)(nil))
)
