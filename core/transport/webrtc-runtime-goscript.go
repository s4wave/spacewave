//go:build goscript

package transport

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	transport_controller "github.com/s4wave/spacewave/net/transport/controller"
	"github.com/sirupsen/logrus"
)

// The goscript browser build excludes the native Pion WebRTC and QUIC
// transports, which pull reflect into the closure through DTLS encoding/gob and
// quic-go. Browser sessions obtain WebRTC/WebSocket transport from the web
// runtime, so the Go-side session transport selector is a no-op here, mirroring
// the tinygo browser build.

func sessionTransportFactories(b bus.Bus) []controller.Factory {
	return nil
}

func (t *SessionTransport) startWebRTCControllers(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
) (*transport_controller.Controller, func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	return nil, nil, nil
}
