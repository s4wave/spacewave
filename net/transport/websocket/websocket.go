package websocket

import (
	"context"
	"net"
	"net/http"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	httplog "github.com/aperturerobotics/util/httplog"
	"github.com/blang/semver/v4"
	"github.com/quic-go/quic-go"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	transport_quic "github.com/s4wave/spacewave/net/transport/common/quic"
	"github.com/s4wave/spacewave/net/util/saddr"
	"github.com/sirupsen/logrus"
)

// TransportType is the transport type identifier for this transport.
const TransportType = "ws"

// ControllerID is the WebSocket controller ID.
const ControllerID = "bifrost/websocket"

// Version is the version of the implementation.
var Version = semver.MustParse("0.0.1")

// WebSocket implements a WebSocket transport.
type WebSocket struct {
	// Transport implements the quic-backed transport type
	*transport_quic.Transport
	// ctx is the context
	ctx context.Context
	// le is the logger
	le *logrus.Entry
	// conf is the websocket config
	conf *Config
}

// NewWebSocket builds a new WebSocket transport.
//
// ServeHTTP is implemented and can be used with a standard HTTP mux.
// Optionally listens on an address.
func NewWebSocket(
	ctx context.Context,
	le *logrus.Entry,
	conf *Config,
	pKey crypto.PrivKey,
	c transport.TransportHandler,
) (*WebSocket, error) {
	peerID, err := peer.IDFromPrivateKey(pKey)
	if err != nil {
		return nil, err
	}

	quicOpts := conf.GetQuic()
	if quicOpts == nil {
		quicOpts = &transport_quic.Opts{}
	} else {
		quicOpts = quicOpts.CloneVT()
	}

	// set websocket-specific quic opts
	quicOpts.DisableDatagrams = true
	quicOpts.DisablePathMtuDiscovery = true

	// websocket manages connection lifecycle, not quic
	quicOpts.DisableKeepAlive = true
	quicOpts.MaxIdleTimeoutDur = "24h"

	tpt := &WebSocket{
		ctx:  ctx,
		le:   le,
		conf: conf,
	}

	var dialFn transport_quic.DialFunc = func(dctx context.Context, addr string) (*quic.Conn, net.Addr, error) {
		conn, _, err := websocket.Dial(dctx, addr, &websocket.DialOptions{
			// Negotiate the bifrost quic sub-protocol ID.
			Subprotocols: []string{transport_quic.Alpn},
		})
		if err != nil {
			return nil, nil, err
		}
		laddr := peer.NewNetAddr(peerID)
		raddr := saddr.NewStringAddr("ws", addr)
		pc := NewPacketConn(ctx, conn, laddr, raddr)
		// Negotiate quic session.
		qconn, _, err := transport_quic.DialSession(ctx, le, quicOpts, pc, tpt.GetIdentity(), raddr, "")
		if err != nil {
			return nil, raddr, err
		}
		return qconn, raddr, nil
	}

	tpt.Transport, err = transport_quic.NewTransport(
		ctx,
		le,
		0,
		nil,
		pKey,
		c,
		quicOpts,
		dialFn,
	)
	if err != nil {
		return nil, err
	}

	return tpt, nil
}

// MatchTransportType checks if the given transport type ID matches this transport.
// If returns true, the transport controller will call DialPeer with that tptaddr.
// E.x.: "udp-quic" or "ws"
func (w *WebSocket) MatchTransportType(transportType string) bool {
	return transportType == TransportType
}

// GetPeerDialer returns the dialing information for a peer.
// Called when resolving EstablishLink.
// Return nil, nil to indicate not found or unavailable.
func (w *WebSocket) GetPeerDialer(ctx context.Context, peerID peer.ID) (*dialer.DialerOpts, error) {
	return w.conf.GetDialers()[peerID.String()], nil
}

// Execute executes the transport as configured, returning any fatal error.
func (w *WebSocket) Execute(ctx context.Context) error {
	// note: w.Transport.Execute is unnecessary (no-op for quic)
	listenAddr := w.conf.GetListenAddr()
	if listenAddr == "" {
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- w.ListenHTTP(ctx, listenAddr)
	}()

	select {
	case <-w.ctx.Done():
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	return nil
}

// ListenHTTP listens for incoming HTTP connections on an address.
func (w *WebSocket) ListenHTTP(ctx context.Context, addr string) error {
	w.le.WithFields(logrus.Fields{
		"addr":    addr,
		"peer-id": w.GetPeerID().String(),
	}).Debug("listening for http/ws")
	server := &http.Server{
		BaseContext: func(net.Listener) context.Context {
			return ctx
		},
		Addr:              addr,
		Handler:           w,
		ReadHeaderTimeout: time.Second * 10,
	}
	err := server.ListenAndServe()
	if serr := server.Shutdown(ctx); serr != nil && serr != context.Canceled {
		w.le.WithError(serr).Warn("graceful shutdown failed")
	}
	_ = server.Close()
	return err
}

// ServeHTTP serves the websocket upgraded HTTP endpoint.
func (w *WebSocket) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	returnNotFound := func() {
		rw.WriteHeader(404)
		_, _ = rw.Write([]byte("404 - Page not found\n"))
		httplog.WithLoggerFields(w.le, req, 404).Debug("request not found")
	}

	httpPath := req.URL.Path
	confPath := w.conf.GetHttpPath()
	httpPeerPath := w.conf.GetHttpPeerPath()

	// Serve peer ID if enabled
	if httpPeerPath != "" && httpPath == httpPeerPath {
		// Serve peer ID
		rw.WriteHeader(200)
		_, _ = rw.Write([]byte(w.GetPeerID().String()))
		return
	}

	// Filter http path if set
	if confPath != "" && req.URL.Path != confPath {
		returnNotFound()
		return
	}

	// Accept websocket
	c, err := websocket.Accept(rw, req, &websocket.AcceptOptions{
		// Negotiate the bifrost quic sub-protocol ID.
		Subprotocols: []string{transport_quic.Alpn},
		// We are trusting the Quic handshake, not the Websocket handshake.
		InsecureSkipVerify: true,
		// We compress on the quic layer instead.
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		httplog.
			WithLoggerFields(w.le, req, 500).
			WithError(err).
			Debug("failed to upgrade websocket")
		rw.WriteHeader(500)
		return
	}

	raddr := saddr.NewStringAddr("ws", req.RemoteAddr)
	pc := NewPacketConn(req.Context(), c, w.LocalAddr(), raddr)
	lnk, err := w.HandleConn(w.ctx, false, pc, raddr, "")
	if err != nil {
		httplog.
			WithLoggerFields(w.le, req, 500).
			WithError(err).
			Debug("failed to handle websocket conn")
		c.Close(websocket.StatusInternalError, "unable to handle connection")
		return
	}

	// hold the HTTP request open until something closes.
	httplog.
		WithLoggerFields(w.le, req, 200).
		Debug("started websocket conn")
	select {
	case <-w.ctx.Done():
	case <-lnk.GetContext().Done():
	case <-req.Context().Done():
	}
	_ = pc.Close()
	_ = lnk.Close()
}

// _ is a type assertion.
var (
	_ transport.Transport    = ((*WebSocket)(nil))
	_ dialer.TransportDialer = ((*WebSocket)(nil))
	_ http.Handler           = ((*WebSocket)(nil))
)
