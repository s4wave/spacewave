package websocket_http

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/crypto"
	bifrost_http "github.com/s4wave/spacewave/net/http"
	"github.com/s4wave/spacewave/net/transport"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

// ControllerID is the WebSocket HTTP handler controller ID.
const ControllerID = "bifrost/websocket/http"

// Version is the version of the implementation.
var Version = semver.MustParse("0.0.1")

// NewWebSocketHttp builds a new WebSocket http handler controller.
type WebSocketHttp struct {
	// Controller is the transport controller
	*transport_controller.Controller
	// mux is the serve mux
	mux *http.ServeMux
}

// NewWebSocketHttp builds a new WebSocket http handler controller.
func NewWebSocketHttp(le *logrus.Entry, b bus.Bus, conf *Config) (*WebSocketHttp, error) {
	peerID, err := conf.ParseTransportPeerID()
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	ctrl := &WebSocketHttp{mux: mux}

	// Ensure no duplicate patterns (causes a panic in net/http)
	seenPatterns := make(map[string]struct{}, len(conf.GetHttpPatterns())+len(conf.GetPeerHttpPatterns()))
	checkPattern := func(httpPattern string) bool {
		if len(httpPattern) == 0 {
			return false
		}

		if _, ok := seenPatterns[httpPattern]; ok {
			le.
				WithField("http-pattern", httpPattern).
				Warn("ignoring duplicate http pattern")
			return false
		}

		seenPatterns[httpPattern] = struct{}{}
		return true
	}

	for _, httpPattern := range conf.GetHttpPatterns() {
		if checkPattern(httpPattern) {
			mux.HandleFunc(httpPattern, ctrl.ServeWebSocketHTTP)
		}
	}

	for _, peerHttpPattern := range conf.GetPeerHttpPatterns() {
		if checkPattern(peerHttpPattern) {
			mux.HandleFunc(peerHttpPattern, ctrl.ServePeerHTTP)
		}
	}

	ctrl.Controller = transport_controller.NewController(
		le,
		b,
		controller.NewInfo(ControllerID, Version, "bifrost websocket http handler"),
		peerID,
		conf.GetVerbose(),
		func(
			ctx context.Context,
			le *logrus.Entry,
			pkey crypto.PrivKey,
			handler transport.TransportHandler,
		) (transport.Transport, error) {
			return websocket.NewWebSocket(
				ctx,
				le,
				&websocket.Config{
					TransportPeerId: peerID.String(),
					Quic:            conf.GetQuic(),
					Dialers:         conf.GetDialers(),
				},
				pkey,
				handler,
			)
		},
	)

	return ctrl, nil
}

// ServeWebSocketHTTP serves the WebSocket on the HTTP response.
func (t *WebSocketHttp) ServeWebSocketHTTP(rw http.ResponseWriter, req *http.Request) {
	// wait for the transport to be ready
	tpt, err := t.GetTransport(req.Context())
	if err != nil {
		// This must be a context canceled error.
		rw.WriteHeader(500)
		return
	}

	// Call ServeHTTP
	tpt.(*websocket.WebSocket).ServeHTTP(rw, req)
}

// ServePeerHTTP serves the peer ID as a string on the HTTP response.
func (t *WebSocketHttp) ServePeerHTTP(rw http.ResponseWriter, req *http.Request) {
	// wait for the transport to be ready
	tpt, err := t.GetTransport(req.Context())
	if err != nil {
		// This must be a context canceled error.
		rw.WriteHeader(500)
		return
	}

	rw.WriteHeader(200)
	_, _ = rw.Write([]byte(tpt.GetPeerID().String())) //nolint:gosec // peer ID is not user-controlled
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The passed context is canceled when the directive instance expires.
// NOTE: the passed context is not canceled when the handler is removed.
func (t *WebSocketHttp) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	switch dir := di.GetDirective().(type) {
	case bifrost_http.LookupHTTPHandler:
		return t.ResolveLookupHTTPHandler(ctx, dir)
	}

	return t.Controller.HandleDirective(ctx, di)
}

// ResolveLookupHTTPHandler resolves the LookupHTTPHandler directive conditionally.
// Returns nil, nil if no handlers matched.
func (t *WebSocketHttp) ResolveLookupHTTPHandler(ctx context.Context, dir bifrost_http.LookupHTTPHandler) ([]directive.Resolver, error) {
	handler, _ := bifrost_http.MatchServeMuxPattern(t.mux, dir)
	if handler == nil {
		return nil, nil
	}

	return directive.R(bifrost_http.NewLookupHTTPHandlerResolver(handler), nil)
}

// _ is a type assertion.
var _ transport.Controller = ((*WebSocketHttp)(nil))
